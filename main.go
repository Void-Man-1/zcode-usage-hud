//go:build windows

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	appName         = "ZCode Usage HUD v" + appVersion
	appBaseName     = "ZCode Usage HUD"
	appVersion      = "1.2.0"
	zcodeAppVersion = "3.11.2"

	balanceURL      = "https://zcode.z.ai/api/v1/zcode-plan/billing/balance?app_version=" + zcodeAppVersion
	customerURL     = "https://api.z.ai/api/biz/customer/getCustomerInfo"
	subscriptionURL = "https://api.z.ai/api/biz/subscription/list"
	quotaURL        = "https://api.z.ai/api/monitor/usage/quota/limit"

	oauthInitURL         = "https://zcode.z.ai/api/v1/oauth/cli/init"
	oauthPollURLPrefix   = "https://zcode.z.ai/api/v1/oauth/cli/poll/"
	oauthBusinessLogin   = "https://api.z.ai/api/auth/z/login"
	oauthDesktopRedirect = "https://zcode.z.ai/app/oauth/login?redirect=zcode%3A%2F%2Foauth%2Fcallback&app_version=" + zcodeAppVersion

	WS_POPUP       = 0x80000000
	WS_VISIBLE     = 0x10000000
	WS_SYSMENU     = 0x00080000
	WS_MINIMIZEBOX = 0x00020000

	WS_EX_TOPMOST = 0x00000008

	WM_NULL          = 0x0000
	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_PAINT         = 0x000F
	WM_ERASEBKGND    = 0x0014
	WM_SETCURSOR     = 0x0020
	WM_CONTEXTMENU   = 0x007B
	WM_MOUSEMOVE     = 0x0200
	WM_LBUTTONDOWN   = 0x0201
	WM_RBUTTONUP     = 0x0205
	WM_TIMER         = 0x0113
	WM_COMMAND       = 0x0111
	WM_SYSCOMMAND    = 0x0112
	WM_NCLBUTTONDOWN = 0x00A1
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_ENTERSIZEMOVE = 0x0231
	WM_MOUSELEAVE    = 0x02A3
	WM_USER          = 0x0400
	NIN_SELECT       = WM_USER
	NIN_KEYSELECT    = WM_USER + 1

	HTCAPTION   = 2
	SC_MINIMIZE = 0xF020

	SW_HIDE       = 0
	SW_SHOWNORMAL = 1
	SW_SHOW       = 5
	SW_RESTORE    = 9

	SWP_NOMOVE     = 0x0002
	SWP_NOSIZE     = 0x0001
	SWP_SHOWWINDOW = 0x0040

	TRANSPARENT = 1

	DT_LEFT         = 0x00000000
	DT_CENTER       = 0x00000001
	DT_RIGHT        = 0x00000002
	DT_VCENTER      = 0x00000004
	DT_SINGLELINE   = 0x00000020
	DT_END_ELLIPSIS = 0x00008000

	MF_STRING       = 0x00000000
	MF_SEPARATOR    = 0x00000800
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	ID_REFRESH  = 1001
	ID_SNAP     = 1002
	ID_STARTUP  = 1003
	ID_OPENZ    = 1004
	ID_EXIT     = 1006
	ID_SHOWHIDE = 1008
	ID_SIGNIN   = 1010
	ID_LOGOUT   = 1011
	ID_IMPORT   = 1012

	SPI_GETWORKAREA = 0x0030

	HKEY_CURRENT_USER    = 0x80000001
	KEY_QUERY_VALUE      = 0x0001
	KEY_SET_VALUE        = 0x0002
	KEY_CREATE_SUB_KEY   = 0x0004
	REG_SZ               = 1
	REG_BINARY           = 3
	REG_DWORD            = 4
	ERROR_FILE_NOT_FOUND = 2
	ERROR_ALREADY_EXISTS = 183

	FW_NORMAL   = 400
	FW_SEMIBOLD = 600

	WM_APP_REFRESH = 0x8001
	WM_APP_EXIT    = 0x8061
	WM_APP_SHOW    = 0x8062
	WM_TRAYICON    = WM_USER + 42

	IDC_ARROW       = 32512
	IDI_APPLICATION = 32512
	IDI_INFORMATION = 32516

	NIM_ADD              = 0x00000000
	NIM_MODIFY           = 0x00000001
	NIM_DELETE           = 0x00000002
	NIM_SETVERSION       = 0x00000004
	NIF_MESSAGE          = 0x00000001
	NIF_ICON             = 0x00000002
	NIF_TIP              = 0x00000004
	NIF_INFO             = 0x00000010
	NIF_SHOWTIP          = 0x00000080
	NIIF_INFO            = 0x00000001
	NOTIFYICON_VERSION_4 = 4
	TME_LEAVE            = 0x00000002
	SRCCOPY              = 0x00CC0020

	CREATE_NO_WINDOW = 0x08000000

	PROCESS_TERMINATE = 0x0001
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procUpdateWindow             = user32.NewProc("UpdateWindow")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procBeginPaint               = user32.NewProc("BeginPaint")
	procEndPaint                 = user32.NewProc("EndPaint")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procTrackMouseEvent          = user32.NewProc("TrackMouseEvent")
	procRegisterWindowMessageW   = user32.NewProc("RegisterWindowMessageW")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procCreateIcon               = user32.NewProc("CreateIcon")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procFillRect                 = user32.NewProc("FillRect")
	procFrameRect                = user32.NewProc("FrameRect")
	procSetBkMode                = gdi32.NewProc("SetBkMode")
	procSetTextColor             = gdi32.NewProc("SetTextColor")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
	procBitBlt                   = gdi32.NewProc("BitBlt")
	procCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procCreateFontW              = gdi32.NewProc("CreateFontW")
	procDrawTextW                = user32.NewProc("DrawTextW")
	procInvalidateRect           = user32.NewProc("InvalidateRect")
	procSetWindowPos             = user32.NewProc("SetWindowPos")
	procSystemParametersInfoW    = user32.NewProc("SystemParametersInfoW")
	procReleaseCapture           = user32.NewProc("ReleaseCapture")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	procAppendMenuW              = user32.NewProc("AppendMenuW")
	procTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	procDestroyMenu              = user32.NewProc("DestroyMenu")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procLoadCursorW              = user32.NewProc("LoadCursorW")
	procLoadIconW                = user32.NewProc("LoadIconW")
	procSetCursor                = user32.NewProc("SetCursor")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procFindWindowExW            = user32.NewProc("FindWindowExW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")
	procGetLastError     = kernel32.NewProc("GetLastError")
	procCloseHandle      = kernel32.NewProc("CloseHandle")
	procOpenProcess      = kernel32.NewProc("OpenProcess")
	procTerminateProcess = kernel32.NewProc("TerminateProcess")

	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegDeleteKeyW    = advapi32.NewProc("RegDeleteKeyW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")

	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	hwndMain          uintptr
	wndProcCallback   = syscall.NewCallback(wndProc)
	arrowCursor       uintptr
	taskbarCreatedMsg uint32
	instanceMutex     uintptr
	refreshPosted     int32

	dataMu          sync.RWMutex
	currentSnapshot Snapshot

	fontHeader          uintptr
	fontBody            uintptr
	fontSmall           uintptr
	fontLabel           uintptr
	fontSection         uintptr
	fontCountdown       uintptr
	fontCountdownSmall  uintptr
	fontStatusCountdown uintptr

	winWidth          = int32(720)
	winHeight         = int32(700)
	snapped           = true
	collapsed         = false
	expandedRectValid bool
	expandedRect      RECT

	// Companion HUD window classes (the installed Codex HUD and its
	// legacy/preview builds). The ZCode collapsed strip stacks above a
	// visible companion instead of overlapping it, since both HUDs
	// default to the same notification-area corner.
	companionHUDClasses = []string{"CodexUsageHUDV3", "CodexUsageHUDPreviewV3", "CodexLimitHUDV2"}
	companionStackGap   = int32(4)

	trayIcon       uintptr
	trayIconCustom bool
	trayAdded      bool
	titleHover     int
	mouseTracking  bool
	previewMode    bool

	// Toasts raised before the tray icon exists are queued here and
	// delivered by flushToastQueue on the UI thread.
	toastMu    sync.Mutex
	toastQueue [][2]string

	// Sign-in button rect, rebuilt on every expanded paint while the
	// account is not usable. Hit-tested in WM_LBUTTONUP.
	signinButtonRect RECT
	// credentials.json mtime at the last fetch; the sign-in watcher
	// refreshes as soon as ZCode writes new tokens.
	lastCredsMod time.Time

	lockStateKnown bool
	lastWasLocked  bool
	lastUnlockAt   time.Time

	fetchMu      sync.Mutex
	fetchBusy    bool
	fetchPending bool
	lastFetch    time.Time
	httpClient   = &http.Client{Timeout: 20 * time.Second}

	// Google sign-in flow state. loginGen supersedes stale flows; only
	// the current generation may publish results.
	loginMu      sync.Mutex
	loginGen     int
	loginPending bool
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}
type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}
type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}
type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}
type NOTIFYICONDATA struct {
	CbSize            uint32
	HWnd              uintptr
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             uintptr
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          GUID
	HBalloonIcon      uintptr
}

// ---- ZCode data model (mirrors /billing/balance + customer APIs) ----

type Entitlement struct {
	EntitlementID string
	ShowName      string
	Meter         string
	UnitType      string
	Capabilities  []string
	GrantUnits    int64
	Period        string
	Priority      int
	// EffectiveAt is when a future grant (e.g. an accepted promotion)
	// starts counting. Zero means immediately effective.
	EffectiveAt time.Time
}

type Plan struct {
	UserPlanID   string
	PlanID       string
	Name         string
	Description  string
	Status       string
	Priority     int
	StartsAt     time.Time
	EndsAt       time.Time
	Entitlements []Entitlement
}

type Balance struct {
	BucketID      string
	ShowName      string
	PlanID        string
	EntitlementID string
	Meter         string
	UnitType      string
	Period        string
	Capabilities  []string
	Total         int64
	Used          int64
	Remaining     int64
	Available     int64
	PeriodStart   time.Time
	PeriodEnd     time.Time
	ExpiresAt     time.Time
}

type Customer struct {
	ID             int64
	CustomerNumber string
	EmailMasked    string
	UserType       string
	Channel        string
	OrgName        string
	ProjectName    string
	IsNewUser      bool
	CreatedAt      string
}

// Subscription is a paid plan subscription (coding plan) from
// subscription/list. Empty for Start-Plan-only accounts. The field
// shapes are parsed defensively — the server has been observed to use
// several key spellings.
type Subscription struct {
	Name     string
	Status   string
	RenewsAt time.Time
}

type Snapshot struct {
	Connected     bool
	SignedIn      bool
	LoginPending  bool
	Email         string
	Name          string
	UserID        string
	Plans         []Plan
	Balances      []Balance
	Subscriptions []Subscription
	Customer      *Customer
	QuotaNote     string
	ServerTime    time.Time
	ServerDrift   time.Duration
	DeviceMid     string
	UpdatedAt     time.Time
	Error         string
}

// ---- balance math (tested) ----

func balanceUsedPercent(b Balance) float64 {
	if b.Total <= 0 {
		return 0
	}
	p := float64(b.Used) / float64(b.Total) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// computeUnlock mirrors the Codex HUD semantics for daily token buckets:
// locked only when every real bucket (total > 0) is fully exhausted.
// Zero-total placeholder buckets neither count as exhausted nor keep the
// HUD available, and the unlock time comes from recurring buckets only —
// an exhausted one-time promo pool never refills, so its period_end must
// not pose as a refill time.
func computeUnlock(bs []Balance, now time.Time) (time.Time, bool) {
	if len(bs) == 0 {
		return time.Time{}, false
	}
	counted := false
	for _, b := range bs {
		if b.Total <= 0 {
			continue // no quota data — ignore for the lock decision
		}
		counted = true
		if b.Remaining > 0 {
			return time.Time{}, false
		}
	}
	if !counted {
		return time.Time{}, false
	}
	var unlock time.Time
	for _, b := range bs {
		if !isRecurringPeriod(b.Period) {
			continue
		}
		if !b.PeriodEnd.IsZero() && b.PeriodEnd.After(unlock) {
			unlock = b.PeriodEnd
		}
	}
	return unlock, true
}

// isRecurringPeriod reports whether a bucket's period refills. One-time
// grants and lifetime pools expire instead — unused tokens are lost at
// period_end. Unknown periods are treated as recurring (the normal
// daily/weekly case) so a new server-side period type keeps refilling.
func isRecurringPeriod(period string) bool {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "one_time", "one-time", "onetime", "lifetime":
		return false
	}
	return true
}

// earliestRefill is the next recurring refill across buckets; one-time
// pools are excluded so the promo expiry never reads as a daily refill.
func earliestRefill(bs []Balance) time.Time {
	var t time.Time
	for _, b := range bs {
		if !isRecurringPeriod(b.Period) {
			continue
		}
		if b.PeriodEnd.IsZero() {
			continue
		}
		if t.IsZero() || b.PeriodEnd.Before(t) {
			t = b.PeriodEnd
		}
	}
	return t
}

// aggregateRemainingPct is the share of token quota left across all real
// buckets (total > 0). The collapsed panel's accent bar uses this
// account-wide number rather than the worst single bucket: one exhausted
// daily bucket must not read as "0% LEFT" while a large promo pool is
// still available. Mirrors computeUnlock, which reports exhausted only
// when everything is gone. Pure data function — unit-testable.
func aggregateRemainingPct(bs []Balance) float64 {
	var total, remaining int64
	for _, b := range bs {
		if b.Total <= 0 {
			continue
		}
		total += b.Total
		remaining += b.Remaining
	}
	if total <= 0 {
		return 100
	}
	p := float64(remaining) / float64(total) * 100
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

func remainingPct(b Balance) float64 {
	if b.Total <= 0 {
		return 100
	}
	p := float64(b.Remaining) / float64(b.Total) * 100
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}

// PendingGrant is an accepted grant that has no balance bucket yet —
// typically a promotion whose effective_at is still in the future (the
// 100M one-time offers land as a new plan + entitlement with no bucket
// until the grant activates). The HUD shows these as upcoming rows so
// the pool is visible before the first token is spendable.
type PendingGrant struct {
	PlanName     string
	ShowName     string
	GrantUnits   int64
	Period       string
	EffectiveAt  time.Time
	PlanEndsAt   time.Time
	Capabilities []string
}

// pendingGrants returns plan entitlements that have no matching balance
// bucket, in plan order. A bucket appears once the grant is effective,
// at which point the grant stops being pending and renders as a normal
// bucket card. Plans whose window has already ended are skipped — their
// grants will never activate. Pure data function — unit-testable.
func pendingGrants(s Snapshot, now time.Time) []PendingGrant {
	liveEnts := map[string]bool{}
	for _, b := range s.Balances {
		if b.EntitlementID != "" {
			liveEnts[b.EntitlementID] = true
		}
	}
	var out []PendingGrant
	for _, p := range s.Plans {
		if !p.EndsAt.IsZero() && !now.Before(p.EndsAt) {
			continue // expired plan — the offer window is closed
		}
		for _, e := range p.Entitlements {
			if e.EntitlementID == "" || liveEnts[e.EntitlementID] {
				continue
			}
			out = append(out, PendingGrant{
				PlanName: p.Name, ShowName: e.ShowName, GrantUnits: e.GrantUnits,
				Period: e.Period, EffectiveAt: e.EffectiveAt, PlanEndsAt: p.EndsAt,
				Capabilities: e.Capabilities,
			})
		}
	}
	return out
}

// activePlan picks the plan to headline: an active plan first, then the
// one ending latest. The API can return several plans; Plans[0] is not
// guaranteed to be the live one.
func (s Snapshot) activePlan() *Plan {
	if len(s.Plans) == 0 {
		return nil
	}
	best := &s.Plans[0]
	for i := range s.Plans[1:] {
		p := &s.Plans[i+1]
		bBest := strings.EqualFold(strings.TrimSpace(best.Status), "active")
		bP := strings.EqualFold(strings.TrimSpace(p.Status), "active")
		if bP && !bBest {
			best = p
			continue
		}
		if bP == bBest && p.EndsAt.After(best.EndsAt) {
			best = p
		}
	}
	return best
}

// periodLabel renders the entitlement period the API reports
// ("daily", "weekly", "monthly", ...) for card titles.
func periodLabel(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "daily":
		return "DAILY"
	case "weekly":
		return "WEEKLY"
	case "monthly":
		return "MONTHLY"
	case "yearly", "annual":
		return "YEARLY"
	case "lifetime":
		return "LIFETIME"
	case "one_time", "one-time", "onetime":
		return "ONE-TIME"
	case "":
		return ""
	default:
		return strings.ToUpper(strings.TrimSpace(period))
	}
}

// periodQualifier builds the short "(daily)" style qualifier for the
// totals row. It only names a period when every bucket shares it.
func periodQualifier(bs []Balance) string {
	if len(bs) == 0 {
		return "(all)"
	}
	first := strings.ToLower(strings.TrimSpace(bs[0].Period))
	if first == "" {
		return "(all)"
	}
	for _, b := range bs[1:] {
		if strings.ToLower(strings.TrimSpace(b.Period)) != first {
			return "(all)"
		}
	}
	return "(" + first + ")"
}

// friendlyQuotaNote keeps the raw Chinese server message out of the HUD:
// a Start-Plan-only account has no coding plan, which is a normal state,
// not an error.
func friendlyQuotaNote(msg string, success bool, level string) string {
	msg = strings.TrimSpace(msg)
	if success {
		if strings.TrimSpace(level) != "" {
			return "Coding-plan level: " + strings.TrimSpace(level)
		}
		return "Coding plan active"
	}
	if msg == "" {
		return "No coding plan"
	}
	return "Start Plan only — no coding plan"
}

// ---- ZCode credential + API layer ----

func zcodeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return filepath.Join(home, ".zcode", "v2")
}

func credentialKey() []byte {
	home, _ := os.UserHomeDir()
	home = strings.TrimRight(home, "\\")
	user := os.Getenv("USERNAME")
	sum := sha256.Sum256([]byte("zcode-credential-fallback:win32:" + home + ":" + user))
	return sum[:]
}

func decryptCredential(s string) (string, error) {
	const prefix = "enc:v1:"
	if !strings.HasPrefix(s, prefix) {
		return s, nil
	}
	parts := strings.Split(s[len(prefix):], ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("bad enc:v1 shape")
	}
	dec := func(p string) ([]byte, error) {
		if m := len(p) % 4; m != 0 {
			p += strings.Repeat("=", 4-m)
		}
		return base64.URLEncoding.DecodeString(p)
	}
	iv, err := dec(parts[0])
	if err != nil {
		return "", err
	}
	tag, err := dec(parts[1])
	if err != nil {
		return "", err
	}
	ct, err := dec(parts[2])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(credentialKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ctAndTag := append(append([]byte{}, ct...), tag...)
	plain, err := gcm.Open(nil, iv, ctAndTag, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

type localCreds struct {
	AccessToken string
	ZCodeJWT    string
	Email       string
	Name        string
	UserID      string
	DeviceMid   string
}

func loadLocalCreds() (localCreds, error) {
	return loadCredsFromFiles(hudCredsPath(), filepath.Join(zcodeDir(), "telemetry-state.json"))
}

// hudCredsPath is the HUD's own session store. It intentionally lives
// next to the HUD's data — not in the ZCode app's credentials.json —
// so a fresh install starts signed out instead of silently attaching
// to whoever is signed in to the ZCode app.
func hudCredsPath() string {
	return filepath.Join(appDataDir(), "credentials.json")
}

func loadCredsFromFiles(credsPath, telemetryPath string) (localCreds, error) {
	var c localCreds
	raw, err := os.ReadFile(credsPath)
	if err != nil {
		return c, err
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return c, err
	}
	if v, ok := m["oauth:zai:access_token"]; ok {
		if d, err := decryptCredential(v); err == nil {
			c.AccessToken = strings.TrimSpace(d)
		}
	}
	if v, ok := m["zcodejwttoken"]; ok {
		if d, err := decryptCredential(v); err == nil {
			c.ZCodeJWT = strings.TrimSpace(d)
		}
	}
	if v, ok := m["oauth:zai:user_info"]; ok {
		if d, err := decryptCredential(v); err == nil {
			var u struct {
				UserID string `json:"user_id"`
				Email  string `json:"email"`
				Name   string `json:"name"`
			}
			if json.Unmarshal([]byte(d), &u) == nil {
				c.Email, c.Name, c.UserID = u.Email, u.Name, u.UserID
			}
		}
	}
	if raw2, err := os.ReadFile(telemetryPath); err == nil {
		var t struct {
			DeviceMid string `json:"deviceMid"`
		}
		if json.Unmarshal(raw2, &t) == nil {
			c.DeviceMid = t.DeviceMid
		}
	}
	return c, nil
}

func zcodeHeaders(token, deviceMid string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "ZCode/"+zcodeAppVersion)
	h.Set("HTTP-Referer", "https://zcode.z.ai")
	h.Set("X-Title", "Z Code@electron")
	h.Set("X-ZCode-App-Version", zcodeAppVersion)
	h.Set("X-Platform", "win32-x64")
	h.Set("X-Os-Category", "windows")
	h.Set("X-Client-Language", "en-US")
	h.Set("X-Client-Timezone", "Europe/Warsaw")
	h.Set("X-Release-Channel", "stable")
	h.Set("X-Os-Version", runtime.GOOS+" "+runtime.GOARCH)
	if deviceMid != "" {
		h.Set("X-Device-Mid", deviceMid)
	}
	if token != "" {
		h.Set("Authorization", "Bearer "+strings.TrimSpace(strings.TrimPrefix(token, "Bearer ")))
	}
	return h
}

func getJSON(url string, headers http.Header, out interface{}) (int, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		if err := dec.Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// getJSONBody is getJSON without decoding — for endpoints whose shape
// is parsed defensively (e.g. subscription lists).
func getJSONBody(url string, headers http.Header) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

type balanceAPIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ServerTime int64 `json:"server_time"`
		Plans      []struct {
			UserPlanID   string `json:"user_plan_id"`
			PlanID       string `json:"plan_id"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Priority     int    `json:"priority"`
			Status       string `json:"status"`
			StartsAt     int64  `json:"starts_at"`
			EndsAt       int64  `json:"ends_at"`
			Entitlements []struct {
				EntitlementID string   `json:"entitlement_id"`
				ShowName      string   `json:"show_name"`
				Meter         string   `json:"meter"`
				UnitType      string   `json:"unit_type"`
				Capabilities  []string `json:"capabilities"`
				GrantUnits    int64    `json:"grant_units"`
				Period        string   `json:"period"`
				Priority      int      `json:"priority"`
				EffectiveAt   int64    `json:"effective_at"`
			} `json:"entitlements"`
		} `json:"plans"`
		Balances []struct {
			BucketID      string   `json:"bucket_id"`
			UserPlanID    string   `json:"user_plan_id"`
			PlanID        string   `json:"plan_id"`
			EntitlementID string   `json:"entitlement_id"`
			ShowName      string   `json:"show_name"`
			Meter         string   `json:"meter"`
			UnitType      string   `json:"unit_type"`
			Capabilities  []string `json:"capabilities"`
			TotalUnits    int64    `json:"total_units"`
			UsedUnits     int64    `json:"used_units"`
			Remaining     int64    `json:"remaining_units"`
			Available     int64    `json:"available_units"`
			PeriodStart   int64    `json:"period_start"`
			PeriodEnd     int64    `json:"period_end"`
			ExpiresAt     int64    `json:"expires_at"`
		} `json:"balances"`
	} `json:"data"`
}

func unixT(v int64) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).Local()
}

func fetchSnapshot() Snapshot {
	now := time.Now()
	creds, err := loadLocalCreds()
	if err != nil {
		if os.IsNotExist(err) {
			// Fresh install: the HUD store doesn't exist yet. This is a
			// normal signed-out state, not an error — never auto-attach
			// to the ZCode app's session.
			return Snapshot{Connected: true, UpdatedAt: now}
		}
		// A present-but-unreadable store (corrupt JSON, permissions) is a
		// reported error state, not "still connecting": Connected=true so
		// the HUD shows OFFLINE instead of pretending to make progress.
		return Snapshot{Connected: true, Error: "Cannot read HUD credentials: " + err.Error(), UpdatedAt: now}
	}
	if creds.ZCodeJWT == "" && creds.AccessToken == "" {
		return Snapshot{Connected: true, UpdatedAt: now}
	}
	s := Snapshot{Connected: true, Email: creds.Email, Name: creds.Name, UserID: creds.UserID, DeviceMid: creds.DeviceMid, UpdatedAt: now}

	// 1) billing/balance carries every token stat: plans, entitlements, buckets.
	if creds.ZCodeJWT != "" {
		var br balanceAPIResponse
		if _, err := getJSON(balanceURL, zcodeHeaders(creds.ZCodeJWT, creds.DeviceMid), &br); err != nil {
			s.Error = "billing/balance failed: " + err.Error()
		} else if br.Code != 0 {
			msg := strings.TrimSpace(br.Msg)
			if msg == "" {
				msg = fmt.Sprintf("code %d", br.Code)
			}
			s.Error = "billing/balance: " + msg
		} else {
			s.SignedIn = true
			if br.Data.ServerTime > 0 {
				s.ServerTime = time.Unix(br.Data.ServerTime, 0).Local()
				s.ServerDrift = now.Sub(s.ServerTime)
			}
			entByID := map[string]struct {
				Period   string
				Priority int
			}{}
			for _, p := range br.Data.Plans {
				plan := Plan{UserPlanID: p.UserPlanID, PlanID: p.PlanID, Name: p.Name, Description: p.Description, Status: p.Status, Priority: p.Priority, StartsAt: unixT(p.StartsAt), EndsAt: unixT(p.EndsAt)}
				for _, e := range p.Entitlements {
					plan.Entitlements = append(plan.Entitlements, Entitlement{EntitlementID: e.EntitlementID, ShowName: e.ShowName, Meter: e.Meter, UnitType: e.UnitType, Capabilities: e.Capabilities, GrantUnits: e.GrantUnits, Period: e.Period, Priority: e.Priority, EffectiveAt: unixT(e.EffectiveAt)})
					if _, ok := entByID[e.EntitlementID]; !ok {
						entByID[e.EntitlementID] = struct {
							Period   string
							Priority int
						}{Period: e.Period, Priority: e.Priority}
					}
				}
				s.Plans = append(s.Plans, plan)
			}
			for _, b := range br.Data.Balances {
				nb := Balance{BucketID: b.BucketID, ShowName: b.ShowName, PlanID: b.PlanID, EntitlementID: b.EntitlementID, Meter: b.Meter, UnitType: b.UnitType, Capabilities: b.Capabilities, Total: b.TotalUnits, Used: b.UsedUnits, Remaining: b.Remaining, Available: b.Available, PeriodStart: unixT(b.PeriodStart), PeriodEnd: unixT(b.PeriodEnd), ExpiresAt: unixT(b.ExpiresAt)}
				if ent, ok := entByID[b.EntitlementID]; ok {
					nb.Period = ent.Period
				}
				s.Balances = append(s.Balances, nb)
			}
			// Highest-grant bucket first (matches the plan's priority
			// order); ShowName breaks ties for stability.
			sort.SliceStable(s.Balances, func(i, j int) bool {
				pi, pj := entByID[s.Balances[i].EntitlementID].Priority, entByID[s.Balances[j].EntitlementID].Priority
				if pi != pj {
					return pi > pj
				}
				return s.Balances[i].ShowName < s.Balances[j].ShowName
			})
		}
	}
	// 2) customer + subscription + coding-plan quota via the Z.AI access token.
	if creds.AccessToken != "" {
		var cr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				ID             int64  `json:"id"`
				CustomerNumber string `json:"customerNumber"`
				Email          string `json:"email"`
				UserType       string `json:"userType"`
				Channel        string `json:"channel"`
				IsNewUser      bool   `json:"isNewUser"`
				CreateTime     string `json:"createTime"`
				Organizations  []struct {
					OrganizationName string `json:"organizationName"`
					Projects         []struct {
						ProjectName string `json:"projectName"`
					} `json:"projects"`
				} `json:"organizations"`
			} `json:"data"`
		}
		if _, err := getJSON(customerURL, zcodeHeaders(creds.AccessToken, creds.DeviceMid), &cr); err == nil && cr.Code == 200 {
			c := &Customer{ID: cr.Data.ID, CustomerNumber: cr.Data.CustomerNumber, EmailMasked: cr.Data.Email, UserType: cr.Data.UserType, Channel: cr.Data.Channel, IsNewUser: cr.Data.IsNewUser, CreatedAt: cr.Data.CreateTime}
			if len(cr.Data.Organizations) > 0 {
				c.OrgName = cr.Data.Organizations[0].OrganizationName
				if len(cr.Data.Organizations[0].Projects) > 0 {
					c.ProjectName = cr.Data.Organizations[0].Projects[0].ProjectName
				}
			}
			s.Customer = c
		}
		// Paid-plan subscriptions (renewal info); empty for
		// Start-Plan-only accounts.
		if raw, err := getJSONBody(subscriptionURL, zcodeHeaders(creds.AccessToken, creds.DeviceMid)); err == nil {
			s.Subscriptions = parseSubscriptions(raw)
		}
		var qr struct {
			Code    int    `json:"code"`
			Msg     string `json:"msg"`
			Success bool   `json:"success"`
			Data    *struct {
				ServerTime int64  `json:"server_time"`
				Level      string `json:"level"`
			} `json:"data"`
		}
		if _, err := getJSON(quotaURL, zcodeHeaders(creds.AccessToken, creds.DeviceMid), &qr); err == nil {
			s.QuotaNote = friendlyQuotaNote(qr.Msg, qr.Success, "")
			if qr.Success && qr.Data != nil {
				s.QuotaNote = friendlyQuotaNote(qr.Msg, true, qr.Data.Level)
			}
		}
	}
	s.UpdatedAt = time.Now()
	return s
}

func refreshNow() {
	fetchMu.Lock()
	if fetchBusy {
		fetchPending = true
		fetchMu.Unlock()
		return
	}
	if !lastFetch.IsZero() && time.Since(lastFetch) < 1500*time.Millisecond {
		fetchMu.Unlock()
		return
	}
	fetchBusy = true
	fetchMu.Unlock()
	go func() {
		s := fetchSnapshot()
		dataMu.Lock()
		currentSnapshot = s
		dataMu.Unlock()
		postRefresh()
		if !previewMode {
			checkPromotionDropped(s)
			checkPromoExpiry(s)
		}
		fetchMu.Lock()
		fetchBusy = false
		pending := fetchPending
		fetchPending = false
		lastFetch = time.Now()
		fetchMu.Unlock()
		if pending {
			time.AfterFunc(2*time.Second, refreshNow)
		}
	}()
}

// parseSubscriptions defensively extracts paid-plan subscriptions from
// subscription/list. The server has been observed to use several field
// spellings, so names/statuses/expiry timestamps are matched loosely.
func parseSubscriptions(body []byte) []Subscription {
	var r struct {
		Code    json.Number              `json:"code"`
		Msg     string                   `json:"msg"`
		Success *bool                    `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return nil
	}
	if r.Success != nil {
		if !*r.Success {
			return nil
		}
	} else if r.Code.String() != "" && r.Code.String() != "200" && r.Code.String() != "0" {
		return nil
	}
	var out []Subscription
	for _, item := range r.Data {
		s := Subscription{Status: "active"}
		for k, v := range item {
			lk := strings.ToLower(k)
			if str, ok := v.(string); ok {
				if s.Name == "" && (strings.Contains(lk, "planname") || strings.Contains(lk, "productname") || lk == "name") {
					s.Name = str
				}
				if lk == "status" && str != "" {
					s.Status = str
				}
			}
			if num, ok := v.(json.Number); ok {
				if n, err := num.Int64(); err == nil && n > 1_000_000_000 {
					// Seconds vs milliseconds: 13-digit values are ms.
					if n > 1_000_000_000_000 {
						n /= 1000
					}
					if strings.Contains(lk, "expire") || strings.Contains(lk, "endtime") ||
						strings.Contains(lk, "end_time") || strings.Contains(lk, "renew") {
						if t := unixT(n); !t.IsZero() && (s.RenewsAt.IsZero() || t.Before(s.RenewsAt)) {
							s.RenewsAt = t
						}
					}
				}
			}
		}
		if s.Name != "" {
			out = append(out, s)
		}
	}
	return out
}

// ---- promotion detection ----
//
// Z.AI drops promotions on the account as new plans/entitlements (the
// 100M one-time offer appears as its own plan with effective_at in the
// future and no bucket until it activates). The HUD diffs every fetch
// against the set of grant keys it has already seen and raises one
// Windows notification per new grant. Keys persist to disk, so a drop
// that happened while the HUD was closed still notifies on next launch,
// and the check uses the HUD's own session — the ZCode app can be off.

type planEntRef struct {
	Plan Plan
	Ent  Entitlement
}

func entitlementGrantKey(userPlanID, entitlementID string) string {
	return userPlanID + "|" + entitlementID
}

// newGrants returns plan/entitlement pairs whose key is not in known.
func newGrants(plans []Plan, known map[string]bool) []planEntRef {
	var out []planEntRef
	for _, p := range plans {
		for _, e := range p.Entitlements {
			if e.EntitlementID == "" {
				continue
			}
			if !known[entitlementGrantKey(p.UserPlanID, e.EntitlementID)] {
				out = append(out, planEntRef{Plan: p, Ent: e})
			}
		}
	}
	return out
}

func promotionsSeenPath() string {
	return filepath.Join(appDataDir(), "promotions-seen.json")
}

func loadPromoSeenKeysAt(path string) (map[string]bool, bool) {
	out := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, false
	}
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		// Corrupt store: treat as absent so the next run re-baselines
		// silently instead of re-toasting every grant as "new".
		return out, false
	}
	for _, k := range keys {
		out[k] = true
	}
	return out, true
}

func savePromoSeenKeysAt(path string, m map[string]bool) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
}

var (
	promoMu      sync.Mutex
	promoKnown   map[string]bool
	promoLoaded  bool
	promoHadFile bool
)

// promoToastBody renders the notification text for a fresh grant.
func promoToastBody(r planEntRef, now time.Time) string {
	body := fmt.Sprintf("%s: %s %s tokens", r.Plan.Name, formatInt64(r.Ent.GrantUnits), r.Ent.ShowName)
	if pl := periodLabel(r.Ent.Period); pl != "" {
		body += " (" + strings.ToLower(pl) + ")"
	}
	if !r.Ent.EffectiveAt.IsZero() {
		if now.Before(r.Ent.EffectiveAt) {
			when := r.Ent.EffectiveAt.Format("15:04")
			if r.Ent.EffectiveAt.After(now.Add(24 * time.Hour)) {
				when = r.Ent.EffectiveAt.Format("02 Jan 15:04")
			}
			body += " · starts " + when
		} else {
			body += " · already active"
		}
	}
	return body
}

// checkPromotionDropped records the account's grant keys and toasts for
// each one not seen before. The first ever run (no persisted store) is a
// silent baseline so an existing Start Plan never notifies.
func checkPromotionDropped(s Snapshot) {
	if !s.SignedIn || len(s.Plans) == 0 {
		return
	}
	promoMu.Lock()
	if !promoLoaded {
		var valid bool
		promoKnown, valid = loadPromoSeenKeysAt(promotionsSeenPath())
		// Only a successfully parsed store counts as a baseline; a
		// missing or corrupt file re-baselines silently.
		promoHadFile = valid
		promoLoaded = true
	}
	var fresh []planEntRef
	if promoHadFile {
		fresh = newGrants(s.Plans, promoKnown)
	}
	for _, p := range s.Plans {
		for _, e := range p.Entitlements {
			if e.EntitlementID != "" {
				promoKnown[entitlementGrantKey(p.UserPlanID, e.EntitlementID)] = true
			}
		}
	}
	promoMu.Unlock()
	if err := savePromoSeenKeysAt(promotionsSeenPath(), promoKnown); err != nil {
		logDiagnostic("promotion seen-store save failed: %v", err)
	}

	now := time.Now()
	if len(fresh) == 1 {
		r := fresh[0]
		logDiagnostic("promotion detected plan=%q entitlement=%s grant=%d", r.Plan.Name, r.Ent.EntitlementID, r.Ent.GrantUnits)
		showTrayNotification("ZCode promotion received", promoToastBody(r, now))
	} else if len(fresh) > 1 {
		// Balloons replace each other on screen, so several grants landing
		// in one refresh become a single summary toast.
		title := fmt.Sprintf("ZCode promotions received (%d)", len(fresh))
		lines := make([]string, 0, len(fresh))
		for _, r := range fresh {
			logDiagnostic("promotion detected plan=%q entitlement=%s grant=%d", r.Plan.Name, r.Ent.EntitlementID, r.Ent.GrantUnits)
			lines = append(lines, promoToastBody(r, now))
		}
		showTrayNotification(title, strings.Join(lines, "\n"))
	}
}

// ---- Google sign-in (ZCode CLI OAuth polling flow) ----
//
// This mirrors the ZCode app's own browser login instead of faking a
// Google form: the HUD asks the backend for a login flow, opens the
// Z.AI authorize page (Google sign-in lives there), polls until the
// user completes it, then resolves and stores the exact same
// credential keys the ZCode app uses.

type oauthInitResult struct {
	flowID       string
	pollToken    string
	pollURL      string
	authorizeURL string
	state        string
	expiresAt    time.Time
	pollInterval time.Duration
}

type oauthReady struct {
	zcodeJWT    string
	oauthAccess string
	userRaw     json.RawMessage
}

// finalizeAuthorizeURL points the server-issued authorize URL at the
// desktop bridge (same redirect the ZCode app uses) and returns the
// final browser URL plus its state parameter.
func finalizeAuthorizeURL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", fmt.Errorf("bad authorize URL")
	}
	q := u.Query()
	q.Set("redirect_uri", oauthDesktopRedirect)
	u.RawQuery = q.Encode()
	state := strings.TrimSpace(u.Query().Get("state"))
	if state == "" {
		return "", "", fmt.Errorf("authorize URL has no state")
	}
	return u.String(), state, nil
}

func parseOAuthInit(body []byte, pollToken string) (oauthInitResult, error) {
	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			FlowID          string `json:"flow_id"`
			AuthorizeURL    string `json:"authorize_url"`
			ExpiresAt       int64  `json:"expires_at"`
			PollIntervalSec int64  `json:"poll_interval_sec"`
		} `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return oauthInitResult{}, err
	}
	if r.Code != 0 {
		msg := strings.TrimSpace(r.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code %d", r.Code)
		}
		return oauthInitResult{}, fmt.Errorf("%s", msg)
	}
	d := r.Data
	if strings.TrimSpace(d.FlowID) == "" || strings.TrimSpace(d.AuthorizeURL) == "" ||
		d.ExpiresAt <= 0 || d.PollIntervalSec <= 0 {
		return oauthInitResult{}, fmt.Errorf("bad init response")
	}
	finalURL, state, err := finalizeAuthorizeURL(d.AuthorizeURL)
	if err != nil {
		return oauthInitResult{}, err
	}
	expiresAt := time.Unix(d.ExpiresAt, 0)
	interval := time.Duration(d.PollIntervalSec) * time.Second
	if interval < time.Second || !time.Now().Before(expiresAt.Add(-interval)) {
		return oauthInitResult{}, fmt.Errorf("bad init response")
	}
	return oauthInitResult{
		flowID: d.FlowID, pollToken: pollToken,
		pollURL:      oauthPollURLPrefix + url.PathEscape(d.FlowID),
		authorizeURL: finalURL, state: state,
		expiresAt: expiresAt, pollInterval: interval,
	}, nil
}

// parseOAuthPoll returns the flow status ("pending", "failed", "ready").
// A non-nil ready holds the completed credentials.
func parseOAuthPoll(body []byte) (string, *oauthReady, error) {
	var r struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return "", nil, err
	}
	if r.Code != 0 || len(r.Data) == 0 {
		msg := strings.TrimSpace(r.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code %d", r.Code)
		}
		return "", nil, fmt.Errorf("%s", msg)
	}
	var d struct {
		Status string          `json:"status"`
		User   json.RawMessage `json:"user"`
		Zai    *struct {
			AccessToken string `json:"access_token"`
		} `json:"zai"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return "", nil, err
	}
	switch strings.TrimSpace(d.Status) {
	case "pending":
		return "pending", nil, nil
	case "failed":
		return "failed", nil, nil
	case "ready":
		access := ""
		if d.Zai != nil {
			access = strings.TrimSpace(d.Zai.AccessToken)
		}
		if strings.TrimSpace(d.Token) == "" || access == "" || len(d.User) == 0 {
			return "", nil, fmt.Errorf("incomplete ready response")
		}
		return "ready", &oauthReady{zcodeJWT: d.Token, oauthAccess: access, userRaw: d.User}, nil
	default:
		return "", nil, fmt.Errorf("unknown status %q", d.Status)
	}
}

// parseBusinessToken extracts the business access token stored as
// oauth:zai:access_token from the api.z.ai auth response.
func parseBusinessToken(body []byte) (string, error) {
	var r struct {
		Code    json.Number `json:"code"`
		Success *bool       `json:"success"`
		Msg     string      `json:"msg"`
		Data    *struct {
			AccessToken  string `json:"access_token"`
			AccessToken2 string `json:"accessToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	ok := true
	if r.Success != nil {
		ok = *r.Success
	} else if r.Code.String() != "" {
		s := r.Code.String()
		ok = s == "0" || s == "200"
	}
	if !ok {
		msg := strings.TrimSpace(r.Msg)
		if msg == "" {
			msg = "business login failed"
		}
		return "", fmt.Errorf("%s", msg)
	}
	tok := ""
	if r.Data != nil {
		tok = strings.TrimSpace(r.Data.AccessToken)
		if tok == "" {
			tok = strings.TrimSpace(r.Data.AccessToken2)
		}
	}
	if tok == "" {
		return "", fmt.Errorf("no business token returned")
	}
	return tok, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// newRequestID mints a UUIDv4 for x-request-id headers.
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// maskSecret redacts a secret for logs, keeping head/tail for correlation.
func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// maskJSONForLog renders a response body safe for hud.log: token-like
// values are redacted, other long strings truncated.
func maskJSONForLog(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var v interface{}
	if json.Unmarshal(raw, &v) != nil {
		s := strings.TrimSpace(string(raw))
		if len(s) > 200 {
			s = s[:200] + "…"
		}
		return s
	}
	maskWalkJSON(v)
	out, _ := json.Marshal(v)
	s := string(out)
	if len(s) > 600 {
		s = s[:600] + "…"
	}
	return s
}

func maskWalkJSON(v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, e := range t {
			if s, ok := e.(string); ok {
				lk := strings.ToLower(k)
				if strings.Contains(lk, "token") || strings.Contains(lk, "secret") ||
					strings.Contains(lk, "password") || (strings.Contains(lk, "code") && len(s) > 12) {
					t[k] = maskSecret(s)
				} else if len(s) > 80 {
					t[k] = s[:80] + "…"
				}
			} else {
				maskWalkJSON(e)
			}
		}
	case []interface{}:
		for _, e := range t {
			maskWalkJSON(e)
		}
	}
}

// encryptCredential seals plaintext in the enc:v1 envelope the ZCode
// app reads (base64url iv.tag.ciphertext, AES-256-GCM).
func encryptCredential(plain string) (string, error) {
	block, err := aes.NewCipher(credentialKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, iv, []byte(plain), nil)
	tagSize := gcm.Overhead()
	ct, tag := sealed[:len(sealed)-tagSize], sealed[len(sealed)-tagSize:]
	enc := base64.RawURLEncoding.EncodeToString
	return "enc:v1:" + enc(iv) + "." + enc(tag) + "." + enc(ct), nil
}

// saveLoginCredentials writes the completed login in the exact key
// layout the ZCode app uses, preserving any unrelated stored keys.
func saveLoginCredentials(zcodeJWT, businessAccess string, userRaw json.RawMessage) error {
	return saveLoginCredentialsAt(hudCredsPath(), zcodeJWT, businessAccess, userRaw)
}

func saveLoginCredentialsAt(path, zcodeJWT, businessAccess string, userRaw json.RawMessage) error {
	m := map[string]string{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &m)
	}
	enc := func(v string) string {
		if s, err := encryptCredential(v); err == nil {
			return s
		}
		return v
	}
	m["oauth:zai:access_token"] = enc(businessAccess)
	m["zcodejwttoken"] = enc(zcodeJWT)
	m["oauth:zai:user_info"] = enc(string(userRaw))
	m["oauth:active_provider"] = enc("zai")
	delete(m, "oauth:zai:refresh_token")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
}

// loginCredentialKeys mirrors the ZCode app's own logout
// (clearProvider("zai") + clearActiveSession): the OAuth access token,
// its refresh slot, the zcode JWT, the cached user profile, and the
// active-provider marker. The app's logout is local-only — there is no
// server revoke endpoint — so ours is too.
var loginCredentialKeys = []string{
	"oauth:zai:access_token",
	"oauth:zai:refresh_token",
	"zcodejwttoken",
	"oauth:zai:user_info",
	"oauth:active_provider",
}

func stripLoginKeys(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, k := range loginCredentialKeys {
		delete(out, k)
	}
	return out
}

func clearLoginCredentials() error {
	return clearLoginCredentialsAt(hudCredsPath())
}

func clearLoginCredentialsAt(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	m := map[string]string{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	out, err := json.MarshalIndent(stripLoginKeys(m), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
}

// signOut cancels any pending login, wipes the HUD's own stored session
// and resets the HUD to the signed-out panel. The ZCode app's separate
// session is never touched.
func signOut() {
	loginMu.Lock()
	loginGen++
	loginPending = false
	loginMu.Unlock()
	if err := clearLoginCredentials(); err != nil {
		logDiagnostic("signout failed: %v", err)
		dataMu.Lock()
		s := currentSnapshot
		s.LoginPending = false
		s.Error = "Sign-out failed: " + err.Error()
		currentSnapshot = s
		dataMu.Unlock()
		postRefresh()
		return
	}
	lastCredsMod = credsModTime()
	logDiagnostic("signed out, HUD login keys cleared")
	dataMu.Lock()
	s := currentSnapshot
	s.SignedIn = false
	s.LoginPending = false
	s.Email, s.Name, s.UserID = "", "", ""
	s.Plans, s.Balances = nil, nil
	s.Customer = nil
	s.QuotaNote = ""
	s.ServerTime = time.Time{}
	s.Error = ""
	s.UpdatedAt = time.Now()
	currentSnapshot = s
	dataMu.Unlock()
	postRefresh()
	showTrayNotification("Signed out", "This HUD's session was cleared.")
	go refreshNow()
}

// importZCodeAppSession copies the ZCode app's current session into the
// HUD's own store (same machine key, so ciphertext transfers verbatim).
// Explicit opt-in only — the HUD never reads the app's store by itself.
func importZCodeAppSession() {
	if err := importZCodeAppSessionFrom(filepath.Join(zcodeDir(), "credentials.json"), hudCredsPath()); err != nil {
		logDiagnostic("session import failed: %v", err)
		dataMu.Lock()
		if !currentSnapshot.SignedIn {
			s := currentSnapshot
			s.Error = err.Error()
			currentSnapshot = s
		}
		dataMu.Unlock()
		postRefresh()
		return
	}
	lastCredsMod = credsModTime()
	logDiagnostic("session imported from ZCode app")
	showTrayNotification("Session imported", "Now using the ZCode app's session.")
	go refreshNow()
}

func importZCodeAppSessionFrom(srcPath, dstPath string) error {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("ZCode app has no stored session — sign in with Google instead.")
	}
	src := map[string]string{}
	if err := json.Unmarshal(raw, &src); err != nil {
		return fmt.Errorf("ZCode app has no stored session — sign in with Google instead.")
	}
	if strings.TrimSpace(src["oauth:zai:access_token"]) == "" || strings.TrimSpace(src["zcodejwttoken"]) == "" {
		return fmt.Errorf("ZCode app has no stored session — sign in with Google instead.")
	}
	dst := map[string]string{}
	if raw2, err := os.ReadFile(dstPath); err == nil {
		_ = json.Unmarshal(raw2, &dst)
	}
	for _, k := range loginCredentialKeys {
		if v, ok := src[k]; ok && strings.TrimSpace(v) != "" {
			dst[k] = v
		}
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(dst, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, append(out, '\n'), 0600)
}

func postJSON(urlStr string, headers http.Header, payload interface{}) (int, []byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	for k := range headers {
		req.Header.Set(k, headers.Get(k))
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, raw, &httpError{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}
	return resp.StatusCode, raw, nil
}

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.status, e.body)
	}
	return fmt.Sprintf("HTTP %d", e.status)
}

// loginAlive reports whether generation gen is still the active login.
func loginAlive(gen int) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	return loginPending && loginGen == gen
}

func setLoginPending(pending bool, errMsg string) {
	dataMu.Lock()
	s := currentSnapshot
	s.LoginPending = pending
	if errMsg != "" {
		s.Error = errMsg
	} else if pending {
		s.Error = ""
	}
	currentSnapshot = s
	dataMu.Unlock()
	postRefresh()
}

func failLogin(gen int, msg string) {
	loginMu.Lock()
	if !(loginPending && loginGen == gen) {
		loginMu.Unlock()
		return
	}
	loginPending = false
	loginMu.Unlock()
	logDiagnostic("signin failed: %s", msg)
	setLoginPending(false, msg)
}

func cancelGoogleLogin() {
	loginMu.Lock()
	loginGen++
	loginPending = false
	loginMu.Unlock()
	logDiagnostic("signin cancelled by user")
	setLoginPending(false, "")
}

// startGoogleLogin runs the full browser flow in the background: init,
// open authorize page, poll to completion, resolve the business token,
// store credentials, refresh.
func startGoogleLogin() {
	loginMu.Lock()
	loginGen++
	gen := loginGen
	loginPending = true
	loginMu.Unlock()
	setLoginPending(true, "")
	logDiagnostic("signin started gen=%d", gen)

	pollToken, err := randomHex(32)
	if err != nil {
		failLogin(gen, "Sign-in failed to start: "+err.Error())
		return
	}
	deviceMid := storedDeviceMid()
	_, raw, err := postJSON(oauthInitURL, zcodeHeaders(pollToken, deviceMid), map[string]string{"provider": "zai"})
	if err != nil {
		if !loginAlive(gen) {
			return
		}
		failLogin(gen, "Sign-in failed to start: "+err.Error())
		return
	}
	initRes, err := parseOAuthInit(raw, pollToken)
	if err != nil {
		if !loginAlive(gen) {
			return
		}
		failLogin(gen, "Sign-in failed to start: "+err.Error())
		return
	}
	logDiagnostic("signin gen=%d browser opened state=%.8s expires=%s", gen, initRes.state, initRes.expiresAt.Format("15:04:05"))
	shellOpen(initRes.authorizeURL)

	for {
		if !loginAlive(gen) {
			return
		}
		if !time.Now().Before(initRes.expiresAt) {
			failLogin(gen, "Sign-in expired — press “Sign in with Google” to try again.")
			return
		}
		time.Sleep(initRes.pollInterval)
		if !loginAlive(gen) {
			return
		}
		status, ready, retryable, err := oauthPoll(initRes.pollURL, initRes.pollToken, deviceMid)
		if err != nil {
			if !loginAlive(gen) {
				return
			}
			if retryable {
				logDiagnostic("signin gen=%d poll retry: %v", gen, err)
				continue
			}
			failLogin(gen, "Sign-in polling failed: "+err.Error())
			return
		}
		switch status {
		case "pending":
			continue
		case "failed":
			failLogin(gen, "Authorization failed in the browser — try again.")
			return
		case "ready":
			finishGoogleLogin(gen, ready)
			return
		default:
			failLogin(gen, "Unexpected sign-in response — try again.")
			return
		}
	}
}

// oauthPoll polls one round. retryable reports whether the caller should
// keep polling (network/5xx/408/429) instead of aborting the flow.
func oauthPoll(pollURL, pollToken, deviceMid string) (string, *oauthReady, bool, error) {
	req, err := http.NewRequest("GET", pollURL, nil)
	if err != nil {
		return "", nil, true, err
	}
	hdrs := zcodeHeaders(pollToken, deviceMid)
	for k := range hdrs {
		req.Header.Set(k, hdrs.Get(k))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, true, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", nil, true, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry := resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500
		return "", nil, retry, &httpError{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
	}
	status, ready, err := parseOAuthPoll(raw)
	if err != nil {
		return "", nil, false, err
	}
	return status, ready, false, nil
}

func finishGoogleLogin(gen int, ready *oauthReady) {
	if !loginAlive(gen) {
		return
	}
	// Mirror the app exactly here: its HTTP client attaches the ZCode
	// header pack only for zcode.z.ai origins. The api.z.ai auth call
	// goes out with Content-Type + x-request-id and nothing else —
	// extra headers have been observed to break this endpoint.
	bizHeaders := http.Header{}
	bizHeaders.Set("x-request-id", newRequestID())
	_, raw, err := postJSON(oauthBusinessLogin, bizHeaders, map[string]string{"token": ready.oauthAccess})
	if err != nil {
		if !loginAlive(gen) {
			return
		}
		logDiagnostic("signin business call failed: %v resp=%s", err, maskJSONForLog(raw))
		failLogin(gen, "Sign-in token exchange failed: "+err.Error())
		return
	}
	business, err := parseBusinessToken(raw)
	if err != nil {
		if !loginAlive(gen) {
			return
		}
		logDiagnostic("signin business parse failed: %v resp=%s", err, maskJSONForLog(raw))
		failLogin(gen, "Sign-in token exchange failed: "+err.Error())
		return
	}
	if err := saveLoginCredentials(ready.zcodeJWT, business, ready.userRaw); err != nil {
		if !loginAlive(gen) {
			return
		}
		failLogin(gen, "Could not save sign-in: "+err.Error())
		return
	}
	loginMu.Lock()
	if !(loginPending && loginGen == gen) {
		loginMu.Unlock()
		return
	}
	loginPending = false
	loginMu.Unlock()
	lastCredsMod = credsModTime()
	logDiagnostic("signin gen=%d completed, credentials saved", gen)
	setLoginPending(false, "")
	showTrayNotification("Signed in to ZCode", "Your token buckets are now live in the HUD.")
	go refreshNow()
}

// ---- app paths / install ----

func main() {
	args := os.Args[1:]
	if containsArg(args, "--uninstall") {
		uninstallApp()
		return
	}
	if containsArg(args, "--dump") {
		// Headless diagnostic: fetch the live snapshot and print it as
		// JSON for debugging (no window is created).
		s := fetchSnapshot()
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(s); err != nil {
			fmt.Fprintln(os.Stderr, "dump failed:", err)
			os.Exit(1)
		}
		return
	}
	if containsArg(args, "--logout") {
		// Headless sign-out: clear the stored session, no window.
		signOut()
		// Give the trailing refresh a moment; signOut itself is done.
		time.Sleep(500 * time.Millisecond)
		fmt.Println("signed out: ZCode login keys cleared")
		return
	}
	if containsArg(args, "--preview") {
		previewMode = true
		runHUD(containsArg(args, "--collapsed"), false)
		return
	}
	if containsArg(args, "--hud") || isInstalledCopy() {
		runHUD(containsArg(args, "--collapsed"), containsArg(args, "--startup"))
		return
	}
	installApp()
}

func containsArg(args []string, wanted string) bool {
	for _, a := range args {
		if strings.EqualFold(a, wanted) {
			return true
		}
	}
	return false
}

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, "Programs", "ZCode Usage HUD")
}
func installedExe() string { return filepath.Join(installDir(), "ZCode Usage HUD.exe") }
func startMenuFolder() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", appBaseName)
}
func mainShortcutPath() string { return filepath.Join(startMenuFolder(), appName+".lnk") }
func uninstallShortcutPath() string {
	return filepath.Join(startMenuFolder(), "Uninstall "+appBaseName+".lnk")
}
func appDataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserHomeDir()
	}
	return filepath.Join(base, "ZCode Usage HUD")
}
func diagnosticLogPath() string { return filepath.Join(appDataDir(), "hud.log") }

func logDiagnostic(format string, args ...interface{}) {
	if err := os.MkdirAll(appDataDir(), 0755); err != nil {
		return
	}
	path := diagnosticLogPath()
	if fi, err := os.Stat(path); err == nil && fi.Size() > 512*1024 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func isRunnableFile(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

func isInstalledCopy() bool {
	cur, err := os.Executable()
	if err != nil {
		return false
	}
	a, _ := filepath.Abs(cur)
	b, _ := filepath.Abs(installedExe())
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func installApp() {
	src, err := os.Executable()
	if err != nil {
		messageBox("Installation failed", err.Error())
		return
	}
	dst := installedExe()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		messageBox("Installation failed", err.Error())
		return
	}

	// Close this HUD and earlier builds before replacing the installed copy.
	closeRunningHUD()
	time.Sleep(300 * time.Millisecond)

	in, err := os.Open(src)
	if err != nil {
		messageBox("Installation failed", err.Error())
		return
	}
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		in.Close()
		messageBox("Installation failed", err.Error())
		return
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	in.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		if copyErr != nil {
			messageBox("Installation failed", copyErr.Error())
		} else {
			messageBox("Installation failed", closeErr.Error())
		}
		return
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		messageBox("Installation failed", err.Error())
		return
	}
	if fi, statErr := os.Stat(dst); statErr != nil || fi.IsDir() || fi.Size() == 0 {
		messageBox("Installation failed", "The installed executable could not be verified at:\n"+dst)
		return
	}

	if err := registerInstalledApp(dst); err != nil {
		messageBox("Installed with warning", "The app was installed, but Windows registration failed:\n"+err.Error())
	}
	if err := createStartMenuShortcuts(dst); err != nil {
		messageBox("Installed with warning", "The application was installed, but its Start Menu shortcuts could not be created:\n"+err.Error()+"\n\nInstalled EXE:\n"+dst)
	}
	startupSummary := "enabled (compact mode)"
	if err := setStartupPath(true, dst); err != nil {
		startupSummary = "NOT enabled — open the HUD menu and choose Start with Windows"
		logDiagnostic("installer could not enable startup: %v", err)
	}

	cmd := exec.Command(dst, "--hud")
	_ = cmd.Start()
	messageBox(appName, "Installed successfully.\n\nApplication folder:\n"+installDir()+"\n\nExecutable:\n"+dst+"\n\nStart Menu folder:\n"+startMenuFolder()+"\n\nStart with Windows: "+startupSummary+"\nNotification-area icon: enabled\nUnlock + promotion notifications: enabled\nMinimize: collapses to a live bucket-preview panel\nClose: exits the HUD\nAccount login: inside the HUD\nUninstall: Windows Settings > Apps > Installed apps")
}

func uninstallApp() {
	closeRunningHUD()
	time.Sleep(300 * time.Millisecond)
	if err := setStartupPath(false, installedExe()); err != nil {
		logDiagnostic("uninstall could not disable startup: %v", err)
	}
	if err := deleteUninstallRegistration(); err != nil {
		logDiagnostic("uninstall could not delete registration: %v", err)
	}
	_ = os.Remove(mainShortcutPath())
	_ = os.Remove(uninstallShortcutPath())
	_ = os.Remove(installDir())
	if err := os.RemoveAll(installDir()); err != nil {
		logDiagnostic("uninstall could not remove install dir: %v", err)
	}
	messageBox(appBaseName, "Removed. The HUD's own data (sessions, logs) stays in:\n"+appDataDir())
}

// registerInstalledApp adds the Add/Remove Programs entry.
func registerInstalledApp(exe string) error {
	key, err := createRegKey(`Software\Microsoft\Windows\CurrentVersion\Uninstall\ZCodeUsageHUD`, KEY_SET_VALUE)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(key)
	if err := regSetString(key, "DisplayName", appBaseName); err != nil {
		return err
	}
	if err := regSetString(key, "DisplayVersion", appVersion); err != nil {
		return err
	}
	if err := regSetString(key, "DisplayIcon", exe); err != nil {
		return err
	}
	if err := regSetString(key, "UninstallString", `"`+filepath.Clean(exe)+`" --uninstall`); err != nil {
		return err
	}
	if err := regSetString(key, "Publisher", appBaseName); err != nil {
		return err
	}
	if err := regSetDWORD(key, "NoModify", 1); err != nil {
		return err
	}
	return regSetDWORD(key, "NoRepair", 1)
}

func hresultFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

var (
	clsidShellLink   = &GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidShellLinkW    = &GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIPersistFile  = &GUID{Data1: 0x0000010B, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	clsctxInprocSrvr = uintptr(1)
	coinitApartment  = uintptr(0x2)
)

// createShellLink writes a .lnk shortcut via the shell COM interfaces.
// Vtable slots: IUnknown QueryInterface=0 Release=2; IShellLinkW
// SetPath=5 SetDescription=7 SetIconLocation=9 SetArguments=12;
// IPersistFile (after IPersist::GetClassID=3) IsDirty=4 Load=5 Save=6.
func createShellLink(linkPath, target, args, description, icon string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	if hr, _, _ := procCoInitializeEx.Call(0, coinitApartment); hresultFailed(hr) {
		return fmt.Errorf("CoInitializeEx failed: 0x%08x", uint32(hr))
	}
	defer procCoUninitialize.Call()

	var shellLink unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsidShellLink)), 0, clsctxInprocSrvr,
		uintptr(unsafe.Pointer(iidShellLinkW)), uintptr(unsafe.Pointer(&shellLink)))
	if hresultFailed(hr) || shellLink == nil {
		return fmt.Errorf("CoCreateInstance failed: 0x%08x", uint32(hr))
	}
	// The object's first field is the vtable pointer; deref into a
	// typed array view so the calls below stay vet-clean.
	linkVtbl := *(**[24]uintptr)(unsafe.Pointer(shellLink))
	defer syscall.SyscallN(linkVtbl[2], uintptr(unsafe.Pointer(shellLink)))

	if setPath, err := syscall.UTF16PtrFromString(filepath.Clean(target)); err == nil {
		if hr, _, _ := syscall.SyscallN(linkVtbl[5], uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(setPath))); hresultFailed(hr) {
			return fmt.Errorf("IShellLink::SetPath failed: 0x%08x", uint32(hr))
		}
	}
	if args != "" {
		if setArgs, err := syscall.UTF16PtrFromString(args); err == nil {
			syscall.SyscallN(linkVtbl[12], uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(setArgs)))
		}
	}
	if description != "" {
		if setDesc, err := syscall.UTF16PtrFromString(description); err == nil {
			syscall.SyscallN(linkVtbl[7], uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(setDesc)))
		}
	}
	if icon != "" {
		if setIcon, err := syscall.UTF16PtrFromString(icon); err == nil {
			syscall.SyscallN(linkVtbl[9], uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(setIcon)), 0)
		}
	}

	var persistFile unsafe.Pointer
	if hr, _, _ := syscall.SyscallN(linkVtbl[0], uintptr(unsafe.Pointer(shellLink)), uintptr(unsafe.Pointer(iidIPersistFile)), uintptr(unsafe.Pointer(&persistFile))); hresultFailed(hr) || persistFile == nil {
		return fmt.Errorf("QueryInterface(IPersistFile) failed: 0x%08x", uint32(hr))
	}
	persistVtbl := *(**[9]uintptr)(unsafe.Pointer(persistFile))
	defer syscall.SyscallN(persistVtbl[2], uintptr(unsafe.Pointer(persistFile)))

	savePath, _ := syscall.UTF16PtrFromString(linkPath)
	if hr, _, _ := syscall.SyscallN(persistVtbl[6], uintptr(unsafe.Pointer(persistFile)), uintptr(unsafe.Pointer(savePath)), 0); hresultFailed(hr) {
		return fmt.Errorf("IPersistFile::Save failed: 0x%08x", uint32(hr))
	}
	return nil
}

func createStartMenuShortcuts(exe string) error {
	if err := os.MkdirAll(startMenuFolder(), 0755); err != nil {
		return err
	}
	// Remove stale versioned shortcuts from previous HUD builds.
	entries, _ := os.ReadDir(startMenuFolder())
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, "zcode usage hud v") && strings.HasSuffix(name, ".lnk") {
			_ = os.Remove(filepath.Join(startMenuFolder(), e.Name()))
		}
	}
	if err := createShellLink(mainShortcutPath(), exe, "--hud", appName, exe); err != nil {
		return err
	}
	if err := createShellLink(uninstallShortcutPath(), exe, "--uninstall", "Remove "+appBaseName, exe); err != nil {
		return err
	}
	return nil
}

func deleteUninstallRegistration() error {
	sub, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Uninstall\ZCodeUsageHUD`)
	r, _, _ := procRegDeleteKeyW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(sub)))
	if r != 0 && r != 2 {
		return fmt.Errorf("RegDeleteKeyW: %d", r)
	}
	return nil
}

func closeRunningHUD() {
	for _, class := range []string{"ZCodeUsageHUDV1", "ZCodeUsageHUDPreviewV1"} {
		cls, _ := syscall.UTF16PtrFromString(class)
		hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
		if hwnd == 0 {
			continue
		}
		// Ask the running HUD to exit asynchronously first, then
		// force-terminate the process only if the window persists.
		procPostMessageW.Call(hwnd, WM_APP_EXIT, 0, 0)
		for i := 0; i < 15; i++ {
			time.Sleep(100 * time.Millisecond)
			remaining, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
			if remaining == 0 {
				hwnd = 0
				break
			}
			hwnd = remaining
		}
		if hwnd == 0 {
			continue
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 {
			continue
		}
		ph, _, _ := procOpenProcess.Call(PROCESS_TERMINATE, 0, uintptr(pid))
		if ph != 0 {
			procTerminateProcess.Call(ph, 0)
			procCloseHandle.Call(ph)
			time.Sleep(250 * time.Millisecond)
		}
	}
}

func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	x, _ := syscall.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(x)), uintptr(unsafe.Pointer(t)), 0x00000040)
}

func previewSnapshot() Snapshot {
	now := time.Now()
	return Snapshot{
		Connected: true, SignedIn: true,
		Email: "account@example.com", Name: "Preview User", UserID: "preview",
		ServerTime: now, DeviceMid: "preview-mid", UpdatedAt: now,
		Plans: []Plan{
			{Name: "ZCode Start Plan", PlanID: "zcode-v3-start-plan-0817", Status: "active", StartsAt: now.Add(-24 * time.Hour), EndsAt: now.Add(4*24*time.Hour + 3*time.Hour)},
			{Name: "ZCode Global Build", PlanID: "zcode-v3-start-plan-0915", Status: "active", StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(22 * time.Hour), Entitlements: []Entitlement{{
				EntitlementID: "ent-preview-promo-100m", ShowName: "GLM-5.3-Flash",
				GrantUnits: 100000000, Period: "one_time", EffectiveAt: now.Add(2*time.Hour + 35*time.Minute),
				Capabilities: []string{"model:glm-5.3-flash"},
			}}},
		},
		Balances: []Balance{
			{ShowName: "GLM-5.3", Total: 3000000, Used: 1140000, Remaining: 1860000, Available: 1860000, PeriodStart: now.Add(-8 * time.Hour), PeriodEnd: nextMidnight(now), ExpiresAt: nextMidnight(now)},
			{ShowName: "GLM-5.3-Flash", Total: 5000000, Used: 4050000, Remaining: 950000, Available: 950000, PeriodStart: now.Add(-8 * time.Hour), PeriodEnd: nextMidnight(now), ExpiresAt: nextMidnight(now)},
		},
		Customer:  &Customer{ID: 12345678, CustomerNumber: "00000000000000000", EmailMasked: "ac****@example.com", UserType: "PERSONAL", Channel: "Z_AI", OrgName: "Default Org", ProjectName: "Default Project"},
		QuotaNote: "No coding plan — Start Plan only",
	}
}

func nextMidnight(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

func runHUD(startCollapsed, fromStartup bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if previewMode {
		publishSnapshot(previewSnapshot())
	} else {
		go refreshNow()
	}
	logDiagnostic("launch version=%s startup=%t collapsed=%t preview=%t", appVersion, fromStartup, startCollapsed, previewMode)

	mutexName := `Local\ZCodeUsageHUD-v1`
	if previewMode {
		mutexName = `Local\ZCodeUsageHUD-preview-v1`
	}
	name, _ := syscall.UTF16PtrFromString(mutexName)
	h, _, _ := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h != 0 {
		instanceMutex = h
		errCode, _, _ := procGetLastError.Call()
		if errCode == ERROR_ALREADY_EXISTS {
			class := "ZCodeUsageHUDV1"
			if previewMode {
				class = "ZCodeUsageHUDPreviewV1"
			}
			cls, _ := syscall.UTF16PtrFromString(class)
			existing, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
			if existing != 0 && !fromStartup {
				procPostMessageW.Call(existing, WM_APP_SHOW, 0, 0)
			}
			logDiagnostic("launch handed off to existing instance startup=%t", fromStartup)
			procCloseHandle.Call(h)
			instanceMutex = 0
			return
		}
	}
	if !previewMode {
		go func() {
			time.Sleep(1500 * time.Millisecond)
			refreshNow()
		}()
	}
	runWindow(startCollapsed)
	if instanceMutex != 0 {
		procCloseHandle.Call(instanceMutex)
		instanceMutex = 0
	}
}

func runWindow(startCollapsed bool) {
	procSetProcessDPIAware.Call()
	hInst, _, _ := procGetModuleHandleW.Call(0)
	arrowCursor, _, _ = procLoadCursorW.Call(0, IDC_ARROW)
	trayIcon = createAppIcon()
	if trayIcon == 0 {
		trayIcon, _, _ = procLoadIconW.Call(0, IDI_INFORMATION)
	} else {
		trayIconCustom = true
	}
	taskbarMsgName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	msgID, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(taskbarMsgName)))
	taskbarCreatedMsg = uint32(msgID)
	class := "ZCodeUsageHUDV1"
	if previewMode {
		class = "ZCodeUsageHUDPreviewV1"
	}
	className, _ := syscall.UTF16PtrFromString(class)
	title, _ := syscall.UTF16PtrFromString(appName)

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   wndProcCallback,
		HInstance:     hInst,
		HIcon:         trayIcon,
		HCursor:       arrowCursor,
		LpszClassName: className,
		HIconSm:       trayIcon,
	}
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logDiagnostic("RegisterClassExW failed")
		return
	}
	style := uintptr(WS_VISIBLE | WS_POPUP | WS_SYSMENU | WS_MINIMIZEBOX)
	hwnd, _, _ := procCreateWindowExW.Call(WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), style,
		100, 100, uintptr(winWidth), uintptr(winHeight), 0, 0, hInst, 0)
	if hwnd == 0 {
		logDiagnostic("CreateWindowExW failed")
		return
	}
	hwndMain = hwnd

	fontHeader = createFont(15, FW_SEMIBOLD)
	fontBody = createFont(10, FW_NORMAL)
	fontSmall = createFont(9, FW_NORMAL)
	fontLabel = createFont(10, FW_SEMIBOLD)
	fontSection = createFont(10, FW_SEMIBOLD)
	fontCountdown = createFont(31, FW_SEMIBOLD)
	fontCountdownSmall = createFont(24, FW_SEMIBOLD)
	fontStatusCountdown = createFont(20, FW_SEMIBOLD)

	addTrayIcon(hwnd)
	flushToastQueue()
	resizeForSnapshot()
	snapToCorner()
	if startCollapsed {
		collapseHUD()
	}
	procSetTimer.Call(hwnd, 1, 1000, 0)
	procSetTimer.Call(hwnd, 2, 60000, 0)
	// Sign-in watcher: while the account is unusable, poll fast and pick
	// up fresh tokens the moment the user signs in via the ZCode app.
	procSetTimer.Call(hwnd, 3, 15000, 0)
	lastCredsMod = credsModTime()
	procShowWindow.Call(hwnd, SW_SHOWNORMAL)
	procUpdateWindow.Call(hwnd)

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	for _, f := range []uintptr{fontHeader, fontBody, fontSmall, fontLabel, fontSection, fontCountdown, fontCountdownSmall, fontStatusCountdown} {
		if f != 0 {
			procDeleteObject.Call(f)
		}
	}
	if trayIconCustom && trayIcon != 0 {
		procDestroyIcon.Call(trayIcon)
		trayIcon = 0
	}
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if taskbarCreatedMsg != 0 && msg == taskbarCreatedMsg {
		trayAdded = false
		addTrayIcon(hwnd)
		return 0
	}
	switch msg {
	case WM_PAINT:
		paint(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_SETCURSOR:
		if arrowCursor != 0 {
			procSetCursor.Call(arrowCursor)
			return 1
		}
	case WM_MOUSEMOVE:
		x, y := mouseXY(lParam)
		if !mouseTracking {
			tme := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: hwnd}
			procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			mouseTracking = true
		}
		newHover := 0
		minRc, closeRc := titleButtonRects(hwnd)
		if pointInRect(x, y, minRc) {
			newHover = 1
		} else if pointInRect(x, y, closeRc) {
			newHover = 2
		}
		if newHover != titleHover {
			titleHover = newHover
			invalidateTitleBar(hwnd)
		}
		return 0
	case WM_MOUSELEAVE:
		mouseTracking = false
		if titleHover != 0 {
			titleHover = 0
			invalidateTitleBar(hwnd)
		}
		return 0
	case WM_LBUTTONDOWN:
		x, y := mouseXY(lParam)
		minRc, closeRc := titleButtonRects(hwnd)
		if !collapsed && y >= 0 && y < 44 && !pointInRect(x, y, minRc) && !pointInRect(x, y, closeRc) {
			snapped = false
			procReleaseCapture.Call()
			procSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
			return 0
		}
	case WM_LBUTTONUP:
		x, y := mouseXY(lParam)
		minRc, closeRc := titleButtonRects(hwnd)
		if pointInRect(x, y, minRc) {
			if collapsed {
				expandHUD()
			} else {
				collapseHUD()
			}
			return 0
		}
		if pointInRect(x, y, closeRc) {
			procDestroyWindow.Call(hwnd)
			return 0
		}
		if collapsed {
			// Anywhere else on the collapsed panel expands it.
			expandHUD()
			return 0
		}
		if signinButtonRect.Right > signinButtonRect.Left && pointInRect(x, y, signinButtonRect) {
			toggleGoogleLogin()
			return 0
		}
	case WM_ENTERSIZEMOVE:
		snapped = false
		return 0
	case WM_RBUTTONUP:
		showMenu(hwnd)
		return 0
	case WM_SYSCOMMAND:
		if uint32(wParam)&0xFFF0 == SC_MINIMIZE {
			collapseHUD()
			return 0
		}
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_APP_EXIT:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_APP_SHOW:
		showHUD()
		return 0
	case WM_TRAYICON:
		event := uint32(lParam & 0xffff)
		switch event {
		case WM_LBUTTONUP, WM_LBUTTONDBLCLK, NIN_SELECT, NIN_KEYSELECT:
			showHUD()
		case WM_RBUTTONUP, WM_CONTEXTMENU:
			showMenu(hwnd)
		}
		return 0
	case WM_TIMER:
		flushToastQueue()
		if wParam == 2 {
			go refreshNow()
		}
		if wParam == 3 {
			// Sign-in watcher: fresh credentials.json from a ZCode
			// sign-in, or a still-unusable account, triggers a fetch.
			if mod := credsModTime(); !mod.IsZero() && mod.After(lastCredsMod) {
				lastCredsMod = mod
				go refreshNow()
			} else {
				dataMu.RLock()
				need := !currentSnapshot.SignedIn
				dataMu.RUnlock()
				if need {
					go refreshNow()
				}
			}
		}
		if collapsed {
			restackCollapsed()
		} else {
			restackExpanded()
		}
		checkUnlockNotification()
		if wParam == 1 && snapshotNeedsOneSecondPaint() {
			invalidateDynamicRegions(hwnd)
		}
		return 0
	case WM_APP_REFRESH:
		atomic.StoreInt32(&refreshPosted, 0)
		resizeForSnapshot()
		checkUnlockNotification()
		flushToastQueue()
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case WM_COMMAND:
		handleCommand(hwnd, int(wParam&0xffff))
		return 0
	case WM_DESTROY:
		removeTrayIcon(hwnd)
		procKillTimer.Call(hwnd, 1)
		procKillTimer.Call(hwnd, 2)
		procKillTimer.Call(hwnd, 3)
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func copyUTF16(dst []uint16, text string) {
	for i := range dst {
		dst[i] = 0
	}
	u, _ := syscall.UTF16FromString(text)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		if len(u) > 0 {
			u[len(u)-1] = 0
		}
	}
	copy(dst, u)
}

func addTrayIcon(hwnd uintptr) {
	if hwnd == 0 || trayAdded {
		return
	}
	if trayIcon == 0 {
		trayIcon, _, _ = procLoadIconW.Call(0, IDI_INFORMATION)
	}
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP | NIF_SHOWTIP
	nid.UCallbackMessage = WM_TRAYICON
	nid.HIcon = trayIcon
	copyUTF16(nid.SzTip[:], appName+" — monitoring ZCode tokens")
	r, _, _ := procShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	trayAdded = r != 0
	if trayAdded {
		nid.UTimeoutOrVersion = NOTIFYICON_VERSION_4
		procShellNotifyIconW.Call(NIM_SETVERSION, uintptr(unsafe.Pointer(&nid)))
	}
}

func removeTrayIcon(hwnd uintptr) {
	if !trayAdded || hwnd == 0 {
		return
	}
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = 1
	procShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	trayAdded = false
}

func showTrayNotification(title, body string) {
	if hwndMain == 0 || !trayAdded {
		// The first fetch can land before the window/tray exist (or the
		// tray icon was recreated after Explorer restart). Queue the toast
		// so it is delivered once the tray is ready instead of being lost.
		toastMu.Lock()
		toastQueue = append(toastQueue, [2]string{title, body})
		if len(toastQueue) > 10 {
			toastQueue = toastQueue[len(toastQueue)-10:]
		}
		toastMu.Unlock()
		return
	}
	sendTrayBalloon(title, body)
}

func sendTrayBalloon(title, body string) {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwndMain
	nid.UID = 1
	nid.UFlags = NIF_INFO
	nid.DwInfoFlags = NIIF_INFO
	copyUTF16(nid.SzInfoTitle[:], title)
	copyUTF16(nid.SzInfo[:], body)
	procShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&nid)))
}

// flushToastQueue delivers toasts queued while the tray was not ready.
// Call from the UI thread once the tray icon exists.
func flushToastQueue() {
	if hwndMain == 0 || !trayAdded {
		return
	}
	toastMu.Lock()
	pending := toastQueue
	toastQueue = nil
	toastMu.Unlock()
	for _, t := range pending {
		sendTrayBalloon(t[0], t[1])
	}
}

func isHUDVisible() bool {
	if hwndMain == 0 {
		return false
	}
	r, _, _ := procIsWindowVisible.Call(hwndMain)
	return r != 0
}

func showHUD() {
	if hwndMain == 0 {
		return
	}
	if !trayAdded {
		addTrayIcon(hwndMain)
	}
	procShowWindow.Call(hwndMain, SW_SHOW)
	procShowWindow.Call(hwndMain, SW_RESTORE)
	if collapsed {
		expandHUD()
	} else {
		procSetWindowPos.Call(hwndMain, ^uintptr(0), 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	}
	procBringWindowToTop.Call(hwndMain)
	procSetForegroundWindow.Call(hwndMain)
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func collapseHUD() {
	if hwndMain == 0 || collapsed {
		return
	}
	var r RECT
	if rr, _, _ := procGetWindowRect.Call(hwndMain, uintptr(unsafe.Pointer(&r))); rr != 0 {
		expandedRect = r
		expandedRectValid = true
	}
	collapsed = true
	titleHover = 0
	mouseTracking = false
	positionCollapsed()
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func expandHUD() {
	if hwndMain == 0 {
		return
	}
	wasCollapsed := collapsed
	collapsed = false
	titleHover = 0
	mouseTracking = false
	if expandedRectValid && !snapped {
		x := expandedRect.Left
		y := expandedRect.Top
		procSetWindowPos.Call(hwndMain, ^uintptr(0), uintptr(x), uintptr(y), uintptr(winWidth), uintptr(winHeight), SWP_SHOWWINDOW)
	} else {
		snapToCorner()
	}
	if wasCollapsed {
		procBringWindowToTop.Call(hwndMain)
		procSetForegroundWindow.Call(hwndMain)
	}
	procInvalidateRect.Call(hwndMain, 0, 0)
}

func toggleHUD() {
	if !isHUDVisible() {
		showHUD()
		return
	}
	if collapsed {
		expandHUD()
	} else {
		collapseHUD()
	}
}

func checkUnlockNotification() {
	dataMu.RLock()
	s := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()
	if !s.SignedIn || len(s.Balances) == 0 {
		lockStateKnown = false
		lastWasLocked = false
		lastUnlockAt = time.Time{}
		return
	}
	now := time.Now()
	unlockAt, locked := computeUnlock(s.Balances, now)
	if !lockStateKnown {
		lockStateKnown = true
		lastWasLocked = locked
		lastUnlockAt = unlockAt
		return
	}
	if locked {
		lastWasLocked = true
		lastUnlockAt = unlockAt
		return
	}
	if lastWasLocked {
		body := "Your ZCode daily token buckets have refilled."
		if !lastUnlockAt.IsZero() {
			body += " Refilled at " + lastUnlockAt.Format("15:04:05") + "."
		}
		showTrayNotification("ZCode tokens available again", body)
		lastWasLocked = false
		lastUnlockAt = time.Time{}
		go refreshNow()
	}
}

func paint(hwnd uintptr) {
	var ps PAINTSTRUCT
	targetDC, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if targetDC == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	width := rc.Right - rc.Left
	height := rc.Bottom - rc.Top
	hdc := targetDC
	var memDC, memBitmap, oldBitmap uintptr
	if width > 0 && height > 0 {
		memDC, _, _ = procCreateCompatibleDC.Call(targetDC)
		if memDC != 0 {
			memBitmap, _, _ = procCreateCompatibleBitmap.Call(targetDC, uintptr(width), uintptr(height))
			if memBitmap != 0 {
				oldBitmap, _, _ = procSelectObject.Call(memDC, memBitmap)
				hdc = memDC
			} else {
				procDeleteDC.Call(memDC)
				memDC = 0
			}
		}
	}
	defer func() {
		if memDC != 0 && memBitmap != 0 {
			procBitBlt.Call(targetDC, 0, 0, uintptr(width), uintptr(height), memDC, 0, 0, SRCCOPY)
			if oldBitmap != 0 {
				procSelectObject.Call(memDC, oldBitmap)
			}
			procDeleteObject.Call(memBitmap)
			procDeleteDC.Call(memDC)
		}
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	}()

	bg := createBrush(rgb(13, 14, 17))
	defer procDeleteObject.Call(bg)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
	procSetBkMode.Call(hdc, TRANSPARENT)
	signinButtonRect = RECT{}

	dataMu.RLock()
	s := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()
	now := time.Now()

	if collapsed {
		paintCollapsed(hdc, rc, s, now)
		return
	}

	const margin int32 = 22
	contentRight := rc.Right - margin

	minRc, closeRc := titleButtonRectsFromClient(rc)
	drawText(hdc, fontHeader, rgb(247, 248, 250), "ZCode Usage", margin, 7, minRc.Left-82, 39, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	versionRc := RECT{minRc.Left - 76, 11, minRc.Left - 12, 34}
	fillPanel(hdc, versionRc, rgb(27, 29, 35))
	framePanel(hdc, versionRc, rgb(42, 45, 54))
	drawText(hdc, fontSmall, rgb(174, 179, 190), "v"+appVersion, versionRc.Left+4, versionRc.Top, versionRc.Right-4, versionRc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	line := RECT{0, 43, rc.Right, 44}
	fillPanel(hdc, line, rgb(34, 36, 43))
	if titleHover == 1 {
		fillPanel(hdc, minRc, rgb(38, 38, 45))
	}
	if titleHover == 2 {
		fillPanel(hdc, closeRc, rgb(196, 57, 67))
	}
	drawText(hdc, fontBody, rgb(210, 210, 219), "—", minRc.Left, minRc.Top, minRc.Right, minRc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	closeColor := rgb(210, 210, 219)
	if titleHover == 2 {
		closeColor = rgb(255, 255, 255)
	}
	drawText(hdc, fontBody, closeColor, "×", closeRc.Left, closeRc.Top, closeRc.Right, closeRc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)

	meta := ""
	if s.Email != "" {
		meta = s.Email
	} else if s.Customer != nil && s.Customer.EmailMasked != "" {
		meta = s.Customer.EmailMasked
	}
	if ap := s.activePlan(); ap != nil && ap.Name != "" {
		if meta != "" {
			meta += "   ·   "
		}
		meta += ap.Name
	}
	if meta == "" && s.Connected {
		meta = "ZCode account not connected"
	}
	if meta != "" {
		dotColor := rgb(111, 115, 126)
		if s.Connected && s.SignedIn {
			dotColor = rgb(91, 201, 128)
		} else if s.Error != "" {
			dotColor = rgb(222, 100, 105)
		}
		dot := RECT{margin, 55, margin + 6, 61}
		fillPanel(hdc, dot, dotColor)
		drawText(hdc, fontSmall, rgb(170, 174, 186), meta, margin+13, 47, contentRight, 69, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}

	y := int32(76)
	signinLabel := "Sign in with Google"
	if s.LoginPending {
		signinLabel = "Waiting for browser sign-in…"
	}
	if !s.Connected {
		panel := RECT{margin, y, contentRight, y + 156}
		fillPanel(hdc, panel, rgb(24, 24, 29))
		drawText(hdc, fontSection, rgb(230, 230, 235), "CONNECT TO ZCODE", margin+14, y+12, contentRight-14, y+34, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		msg := s.Error
		if msg == "" {
			msg = "Reading %USERPROFILE%\\.zcode\\v2\\credentials.json…"
		}
		drawText(hdc, fontBody, rgb(207, 207, 214), msg, margin+14, y+42, contentRight-14, y+68, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(hdc, fontSmall, rgb(125, 125, 137), "Opens Z.AI login in your browser — choose Google there.", margin+14, y+72, contentRight-14, y+94, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		signinButtonRect = RECT{margin + 16, y + 102, margin + 330, y + 142}
		fillPanel(hdc, signinButtonRect, rgb(47, 47, 57))
		drawText(hdc, fontLabel, rgb(235, 235, 240), signinLabel, signinButtonRect.Left+8, signinButtonRect.Top, signinButtonRect.Right-8, signinButtonRect.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	} else if !s.SignedIn {
		panel := RECT{margin, y, contentRight, y + 196}
		fillPanel(hdc, panel, rgb(24, 24, 29))
		drawText(hdc, fontSection, rgb(230, 230, 235), "ZCODE SIGN-IN REQUIRED", margin+16, y+14, contentRight-16, y+36, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		msg := s.Error
		if msg == "" {
			if s.LoginPending {
				msg = "Complete the Google sign-in in your browser…"
			} else {
				msg = "Not signed in yet — use the button below or the tray menu."
			}
		}
		drawText(hdc, fontBody, rgb(207, 207, 214), msg, margin+16, y+44, contentRight-16, y+68, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(hdc, fontSmall, rgb(135, 135, 146), "New tokens are picked up automatically when done.", margin+16, y+74, contentRight-16, y+98, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		if s.QuotaNote != "" {
			drawText(hdc, fontSmall, rgb(220, 125, 125), s.QuotaNote, margin+16, y+102, contentRight-16, y+124, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		}
		signinButtonRect = RECT{margin + 16, y + 132, margin + 330, y + 172}
		fillPanel(hdc, signinButtonRect, rgb(47, 47, 57))
		drawText(hdc, fontLabel, rgb(235, 235, 240), signinLabel, signinButtonRect.Left+8, signinButtonRect.Top, signinButtonRect.Right-8, signinButtonRect.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	} else {
		unlockAt, locked := computeUnlock(s.Balances, now)
		status := RECT{margin, y, contentRight, y + 90}
		fillPanel(hdc, status, rgb(22, 24, 29))
		framePanel(hdc, status, rgb(35, 38, 46))
		if locked {
			accent := RECT{status.Left, status.Top, status.Left + 4, status.Bottom}
			fillPanel(hdc, accent, rgb(232, 167, 76))
			drawText(hdc, fontSection, rgb(242, 182, 94), "TOKENS EXHAUSTED", margin+18, y+8, contentRight-170, y+30, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
			drawText(hdc, fontSmall, rgb(159, 164, 176), "REFILLS IN", margin+190, y+8, contentRight-16, y+28, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
			countdown := durationClock(unlockAt.Sub(now))
			unlockText := "Refills " + unlockAt.Format("Mon 02 Jan · 15:04:05") + " local"
			if unlockAt.IsZero() {
				countdown = "Not reported"
				unlockText = "No recurring tokens left — one-time pools do not refill"
			} else if !now.Before(unlockAt) {
				countdown = "Confirming refill…"
				unlockText = "Waiting for fresh balance data"
			}
			drawText(hdc, fontCountdownSmall, rgb(248, 248, 250), countdown, margin+18, y+30, contentRight-16, y+68, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
			drawText(hdc, fontSmall, rgb(155, 160, 173), unlockText, margin+18, y+67, contentRight-16, y+88, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		} else {
			accent := RECT{status.Left, status.Top, status.Left + 4, status.Bottom}
			fillPanel(hdc, accent, rgb(82, 193, 122))
			availableText := "AVAILABLE"
			if len(s.Balances) == 0 {
				availableText = "NO BUCKETS REPORTED"
			}
			drawText(hdc, fontSection, rgb(122, 220, 155), availableText, margin+18, y+11, contentRight-170, y+34, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
			drawText(hdc, fontSmall, rgb(151, 157, 170), "SYNCED "+syncText(s.UpdatedAt), contentRight-170, y+11, contentRight-16, y+33, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
			refill := earliestRefill(s.Balances)
			body := "Token buckets have remaining quota."
			if !refill.IsZero() {
				body = "Next recurring refill in " + durationClock(refill.Sub(now)) + " (" + refill.Format("15:04") + " local)."
			}
			drawText(hdc, fontBody, rgb(224, 227, 233), body, margin+18, y+38, contentRight-16, y+62, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		}
		y += 94

		drawText(hdc, fontSection, rgb(194, 194, 203), "TOKEN BUCKETS", margin, y, contentRight, y+24, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		y += 30

		if len(s.Balances) == 0 {
			panel := RECT{margin, y, contentRight, y + 70}
			fillPanel(hdc, panel, rgb(24, 24, 29))
			drawText(hdc, fontBody, rgb(213, 213, 221), "No token buckets were returned for this account yet.", margin+14, y+12, contentRight-14, y+38, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
			drawText(hdc, fontSmall, rgb(130, 130, 142), "Last sync: "+syncText(s.UpdatedAt), margin+14, y+40, contentRight-14, y+62, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
			y += 82
		} else {
			for _, b := range s.Balances {
				drawBalanceCard(hdc, &rc, b, y, now)
				y += 146
			}
		}

		if pend := pendingGrants(s, now); len(pend) > 0 {
			y += 6
			drawText(hdc, fontSection, rgb(194, 194, 203), "PROMO GRANTS · NOT SPENDABLE YET", margin, y, contentRight, y+24, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
			y += 30
			for _, g := range pend {
				drawPendingGrantCard(hdc, &rc, g, y, now)
				y += 146
			}
		}

		statsTop := y
		statsBottom := statsTop + 196 + 24 // +24: subscription line
		if extra := len(s.Plans) - 1; extra > 0 {
			statsBottom += int32(extra) * 24
		}
		if statsBottom < rc.Bottom-30 {
			stats := RECT{margin, statsTop, contentRight, statsBottom}
			fillPanel(hdc, stats, rgb(24, 24, 29))
			drawStatsPanel(hdc, stats, s, now)
			y = statsBottom + 8
		}
	}

	footer := connectionFooter(s)
	drawText(hdc, fontSmall, rgb(126, 132, 146), footer, margin, rc.Bottom-26, contentRight, rc.Bottom-6, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func drawCompactStatusCountdown(hdc uintptr, color uint32, d time.Duration, left, top, right, bottom int32) {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	days := total / 86400
	total %= 86400
	h := total / 3600
	total %= 3600
	m := total / 60
	sec := total % 60
	clock := fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
	const dayNumberW int32 = 62
	const daySuffixW int32 = 14
	const gap int32 = 8
	const clockW int32 = 112
	totalW := dayNumberW + daySuffixW + gap + clockW
	center := (left + right) / 2
	start := center - totalW/2
	if start < left {
		start = left
	}
	dayNumberRc := RECT{start, top, start + dayNumberW, bottom}
	daySuffixRc := RECT{dayNumberRc.Right, top, dayNumberRc.Right + daySuffixW, bottom}
	clockRc := RECT{daySuffixRc.Right + gap, top, daySuffixRc.Right + gap + clockW, bottom}
	if clockRc.Right > right {
		delta := clockRc.Right - right
		dayNumberRc.Left -= delta
		dayNumberRc.Right -= delta
		daySuffixRc.Left -= delta
		daySuffixRc.Right -= delta
		clockRc.Left -= delta
		clockRc.Right -= delta
	}
	if days > 0 {
		drawText(hdc, fontStatusCountdown, color, fmt.Sprintf("%d", days), dayNumberRc.Left, dayNumberRc.Top, dayNumberRc.Right, dayNumberRc.Bottom, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, fontStatusCountdown, color, "d", daySuffixRc.Left, daySuffixRc.Top, daySuffixRc.Right, daySuffixRc.Bottom, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
	drawText(hdc, fontStatusCountdown, color, clock, clockRc.Left, clockRc.Top, clockRc.Right, clockRc.Bottom, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

// ---- promo expiry warnings ----
//
// One-time pools are temporary: whatever is unused when period_end hits
// is lost. Warn once per bucket when a non-recurring pool with tokens
// still in it enters its final 24 hours, so a pool never silently dies
// overnight. Keys persist so each (bucket, expiry) pair warns only once.

func expiringPromos(bs []Balance, now time.Time, within time.Duration) []Balance {
	var out []Balance
	for _, b := range bs {
		if isRecurringPeriod(b.Period) || b.PeriodEnd.IsZero() || b.Remaining <= 0 {
			continue
		}
		if now.Before(b.PeriodEnd) && b.PeriodEnd.Sub(now) <= within {
			out = append(out, b)
		}
	}
	return out
}

func expirySeenPath() string {
	return filepath.Join(appDataDir(), "expiry-notified.json")
}

func loadExpirySeenKeysAt(path string) (map[string]bool, bool) {
	out := map[string]bool{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, false
	}
	var keys []string
	if err := json.Unmarshal(raw, &keys); err != nil {
		return out, false
	}
	for _, k := range keys {
		out[k] = true
	}
	return out, true
}

func saveExpirySeenKeysAt(path string, m map[string]bool) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0600)
}

func expiryKey(b Balance) string {
	return b.BucketID + "|" + strconv.FormatInt(b.PeriodEnd.Unix(), 10)
}

// checkPromoExpiry warns about one-time pools about to expire.
func checkPromoExpiry(s Snapshot) {
	if !s.SignedIn || len(s.Balances) == 0 {
		return
	}
	now := time.Now()
	expiring := expiringPromos(s.Balances, now, 24*time.Hour)
	if len(expiring) == 0 {
		return
	}
	path := expirySeenPath()
	// A missing or corrupt store just means warnings may repeat once —
	// the save below re-persists the full set either way.
	seen, _ := loadExpirySeenKeysAt(path)
	var fresh []Balance
	for _, b := range expiring {
		k := expiryKey(b)
		if !seen[k] {
			fresh = append(fresh, b)
			seen[k] = true
		}
	}
	if len(fresh) == 0 {
		return
	}
	if err := saveExpirySeenKeysAt(path, seen); err != nil {
		logDiagnostic("expiry seen-store save failed: %v", err)
	}
	for _, b := range fresh {
		logDiagnostic("promo expiring bucket=%s end=%s remaining=%d", b.BucketID, b.PeriodEnd.Format("15:04"), b.Remaining)
		showTrayNotification("ZCode promo pool expiring", fmt.Sprintf(
			"%s: %s tokens unused · expires %s local (in %s) — unused tokens are lost",
			b.ShowName, formatInt64(b.Remaining), b.PeriodEnd.Format("Mon 02 Jan 15:04"), durationClock(b.PeriodEnd.Sub(now))))
	}
}

// bucketQualifier names what kind of quota a bucket holds: one-time
// grants are promotions, recurring periods are the plan's regular
// quota. Rendered next to the model name so the two are never confused.
func bucketQualifier(b Balance) (string, uint32) {
	if !isRecurringPeriod(b.Period) {
		return "PROMO", rgb(96, 165, 250)
	}
	switch strings.ToLower(strings.TrimSpace(b.Period)) {
	case "daily":
		return "DAILY", rgb(150, 155, 168)
	case "weekly":
		return "WEEKLY", rgb(150, 155, 168)
	case "monthly":
		return "MONTHLY", rgb(150, 155, 168)
	case "yearly", "annual":
		return "YEARLY", rgb(150, 155, 168)
	}
	return "QUOTA", rgb(150, 155, 168)
}

// drawBucketRows renders the collapsed panel's stacked rows: one
// full-width row per token bucket, no abbreviations — the full model
// name with a PROMO / DAILY / WEEKLY qualifier on the first line, and
// that bucket's own green/amber/red progress bar with its remaining %
// underneath. Pending promotion grants (accepted, not yet spendable)
// share the layout with an amber activation countdown instead of a
// usage bar. Rows start below the slim top-right button strip and span
// the full panel width; rows beyond four collapse into a "+N more" row.
func drawBucketRows(hdc uintptr, bs []Balance, pend []PendingGrant, left, top, right, bottom int32) {
	const rowH, gap int32 = 34, 4
	total := len(bs) + len(pend)
	shown := total
	if shown > 4 {
		shown = 4
	}
	y := top + 24 // below the compact top-right button strip
	for i := 0; i < shown; i++ {
		rowTop := y + int32(i)*(rowH+gap)
		rowBottom := rowTop + rowH
		if rowBottom > bottom {
			break
		}
		if i == shown-1 && shown < total {
			drawText(hdc, fontSmall, rgb(150, 155, 168), fmt.Sprintf("+%d more rows", total-shown), left, rowTop, right, rowBottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
			continue
		}
		if i < len(bs) {
			drawBucketRow(hdc, bs[i], left, right, rowTop, rowBottom)
		} else {
			drawPendingRow(hdc, pend[i-len(bs)], left, right, rowTop, rowBottom)
		}
	}
}

func drawBucketRow(hdc uintptr, b Balance, left, right, rowTop, rowBottom int32) {
	pct := remainingPct(b)
	pctColor := rgb(151, 224, 169)
	barColor := rgb(75, 170, 110)
	if pct < 10 {
		pctColor = rgb(240, 148, 126)
		barColor = rgb(205, 75, 75)
	} else if pct < 25 {
		pctColor = rgb(242, 182, 94)
		barColor = rgb(205, 157, 57)
	}
	qual, qualColor := bucketQualifier(b)
	// Line 1: full model name left, quota kind right.
	drawText(hdc, fontLabel, rgb(225, 228, 235), b.ShowName, left, rowTop, right-64, rowTop+16, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontSmall, qualColor, qual, right-60, rowTop, right, rowTop+16, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
	// Line 2: full-width bar, remaining % right.
	barTop := rowTop + 19
	barBottom := rowTop + 27
	barRight := right - 52
	if barRight > left {
		bar := RECT{left, barTop, barRight, barBottom}
		fillPanel(hdc, bar, rgb(43, 46, 55))
		fillW := int32(float64(bar.Right-bar.Left) * pct / 100)
		if fillW > 0 {
			fillPanel(hdc, RECT{bar.Left, bar.Top, bar.Left + fillW, bar.Bottom}, barColor)
		}
	}
	drawText(hdc, fontSmall, pctColor, fmt.Sprintf("%.0f%%", pct), right-46, rowTop+15, right, rowBottom, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
}

// drawPendingRow renders a promotion grant that has no bucket yet: the
// second line carries an amber activation countdown and the grant size
// instead of a usage bar.
func drawPendingRow(hdc uintptr, g PendingGrant, left, right, rowTop, rowBottom int32) {
	drawText(hdc, fontLabel, rgb(225, 228, 235), g.ShowName, left, rowTop, right-64, rowTop+16, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontSmall, rgb(96, 165, 250), "PROMO", right-60, rowTop, right, rowTop+16, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
	status := "WAITING FOR ACTIVATION"
	if !g.EffectiveAt.IsZero() {
		if time.Now().Before(g.EffectiveAt) {
			status = "STARTS IN " + durationClock(g.EffectiveAt.Sub(time.Now()))
		} else {
			status = "ACTIVATING…"
		}
	}
	drawText(hdc, fontSmall, rgb(242, 182, 94), status, left, rowTop+19, right-100, rowBottom, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontSmall, rgb(242, 182, 94), formatInt64(g.GrantUnits), right-96, rowTop+19, right, rowBottom, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func paintCollapsed(hdc uintptr, rc RECT, s Snapshot, now time.Time) {
	minRc, closeRc := titleButtonRectsFromClient(rc)
	if titleHover == 1 {
		fillPanel(hdc, minRc, rgb(40, 40, 47))
	}
	if titleHover == 2 {
		fillPanel(hdc, closeRc, rgb(196, 57, 67))
	}
	text := "—"
	color := rgb(205, 205, 214)
	accent := rgb(89, 89, 102)
	var countdown time.Duration
	hasCountdown := false
	var previewBuckets []Balance
	var previewPendings []PendingGrant
	if !s.Connected {
		text = "CONNECTING"
	} else if !s.SignedIn {
		// Distinguish "no account yet" from "account ok but API unreachable".
		if s.Error != "" {
			text = "OFFLINE"
			color = rgb(240, 148, 126)
		} else {
			text = "SIGN IN"
		}
	} else {
		unlockAt, locked := computeUnlock(s.Balances, now)
		if locked {
			countdown = unlockAt.Sub(now)
			hasCountdown = !unlockAt.IsZero() && now.Before(unlockAt)
			if !hasCountdown {
				text = "CHECKING REFILL"
			}
			color = rgb(248, 248, 250)
			accent = rgb(232, 167, 76)
		} else {
			// Accent bar tracks the account-wide remaining share so a
			// nearly-dry account reads as a warning even before the
			// per-bucket preview is examined.
			pct := aggregateRemainingPct(s.Balances)
			text = fmt.Sprintf("%.0f%% LEFT", pct)
			if pct < 10 {
				color = rgb(240, 148, 126)
				accent = rgb(205, 75, 75)
			} else if pct < 25 {
				color = rgb(242, 182, 94)
				accent = rgb(232, 167, 76)
			} else {
				color = rgb(151, 224, 169)
				accent = rgb(90, 190, 121)
			}
			for _, b := range s.Balances {
				if b.Total > 0 {
					previewBuckets = append(previewBuckets, b)
				}
			}
			previewPendings = pendingGrants(s, now)
		}
	}
	accentRc := RECT{0, 0, 3, rc.Bottom}
	fillPanel(hdc, accentRc, accent)
	if hasCountdown {
		drawCompactStatusCountdown(hdc, color, countdown, 8, 0, minRc.Left-6, rc.Bottom)
	} else if len(previewBuckets) > 0 || len(previewPendings) > 0 {
		// Stacked rows: one full-width line per bucket or pending
		// promotion grant — bars run the whole panel width since the
		// buttons live in their own top strip.
		drawBucketRows(hdc, previewBuckets, previewPendings, 10, 0, rc.Right-8, rc.Bottom)
	} else {
		drawText(hdc, fontStatusCountdown, color, text, 8, 0, minRc.Left-6, rc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	}
	restoreColor := rgb(211, 211, 220)
	drawText(hdc, fontBody, restoreColor, "□", minRc.Left, minRc.Top, minRc.Right, minRc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	closeColor := rgb(211, 211, 220)
	if titleHover == 2 {
		closeColor = rgb(255, 255, 255)
	}
	drawText(hdc, fontBody, closeColor, "×", closeRc.Left, closeRc.Top, closeRc.Right, closeRc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

func fillPanel(hdc uintptr, rc RECT, color uint32) {
	br := createBrush(color)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	procDeleteObject.Call(br)
}

func framePanel(hdc uintptr, rc RECT, color uint32) {
	br := createBrush(color)
	procFrameRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), br)
	procDeleteObject.Call(br)
}

func drawStatsPanel(hdc uintptr, panel RECT, s Snapshot, now time.Time) {
	left := panel.Left + 14
	right := panel.Right - 14
	mid := (left + right) / 2
	framePanel(hdc, panel, rgb(35, 38, 46))
	drawText(hdc, fontSection, rgb(211, 215, 224), "ACCOUNT · PLAN · TOKENS", left, panel.Top+8, right, panel.Top+30, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	separator := RECT{left, panel.Top + 34, right, panel.Top + 35}
	fillPanel(hdc, separator, rgb(37, 40, 48))
	extraPlans := len(s.Plans) - 1
	if extraPlans < 0 {
		extraPlans = 0
	}
	divider := RECT{mid, panel.Top + 42, mid + 1, panel.Top + 134 + int32(extraPlans)*24}
	fillPanel(hdc, divider, rgb(37, 40, 48))

	var totalGrant, totalUsed, totalLeft int64
	for _, b := range s.Balances {
		totalGrant += b.Total
		totalUsed += b.Used
		totalLeft += b.Remaining
	}
	row := panel.Top + 40
	drawStatPair(hdc, left, mid-8, "Buckets", fmt.Sprintf("%d", len(s.Balances)), row)
	planName := "—"
	if ap := s.activePlan(); ap != nil && ap.Name != "" {
		planName = ap.Name
	}
	// Full names — the stat value ellipsizes at pixel width, so no
	// character-level truncation here.
	drawStatPair(hdc, mid+8, right, "Plan", planName, row)
	row += 25
	qual := periodQualifier(s.Balances)
	drawStatPair(hdc, left, mid-8, "Granted "+qual, formatInt64(totalGrant), row)
	drawStatPair(hdc, mid+8, right, "Used "+qual, formatInt64(totalUsed), row)
	row += 25
	drawStatPair(hdc, left, mid-8, "Remaining", formatInt64(totalLeft), row)
	custText := "—"
	if s.Customer != nil {
		custText = fmt.Sprintf("#%d", s.Customer.ID)
	}
	drawStatPair(hdc, mid+8, right, "Customer", custText, row)
	row += 25
	orgText := "—"
	if s.Customer != nil && s.Customer.OrgName != "" {
		orgText = s.Customer.OrgName
		if s.Customer.ProjectName != "" {
			orgText += " · " + s.Customer.ProjectName
		}
	}
	drawStatPair(hdc, left, mid-8, "Org · Project", orgText, row)
	quota := s.QuotaNote
	if quota == "" {
		quota = "—"
	}
	drawStatPair(hdc, mid+8, right, "Coding plan", quota, row)
	row = panel.Top + 143

	// Every plan the account reports, newest/most-relevant first: the
	// active plan headlines with its expiry countdown; any other plan
	// (expired, or a promotion with its own window) gets its own line.
	planLine := "No active plan reported"
	if ap := s.activePlan(); ap != nil && !ap.EndsAt.IsZero() {
		if now.Before(ap.EndsAt) {
			planLine = "ends " + ap.EndsAt.Format("02 Jan 15:04") + " · in " + durationClock(ap.EndsAt.Sub(now))
		} else {
			planLine = "ended " + ap.EndsAt.Format("02 Jan 15:04")
		}
	}
	drawText(hdc, fontSmall, rgb(165, 170, 183), "Plan expiry", left, row, left+112, row+22, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fontBody, rgb(231, 233, 238), planLine, left+118, row, right, row+22, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	row += 24
	ap := s.activePlan()
	for i := range s.Plans {
		p := &s.Plans[i]
		// The headlined plan already has its expiry line; skip it by
		// pointer identity (UserPlanID can be empty, so IDs are not a
		// reliable comparison).
		if ap != nil && p == ap {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.PlanID
		}
		state := strings.ToLower(strings.TrimSpace(p.Status))
		if state == "" {
			state = "?"
		}
		when := ""
		if !p.EndsAt.IsZero() {
			if now.Before(p.EndsAt) {
				when = " · ends " + p.EndsAt.Format("02 Jan 15:04") + " · in " + durationClock(p.EndsAt.Sub(now))
			} else {
				when = " · ended " + p.EndsAt.Format("02 Jan 15:04")
			}
		}
		if !p.StartsAt.IsZero() && now.Before(p.StartsAt) {
			when = " · starts " + p.StartsAt.Format("02 Jan 15:04")
		}
		drawText(hdc, fontSmall, rgb(140, 145, 158), name, left, row, left+196, row+22, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		drawText(hdc, fontSmall, rgb(198, 202, 212), state+when, left+202, row, right, row+22, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		row += 24
	}
	// Paid-plan subscriptions (empty for Start-Plan-only accounts).
	subsLine := "—"
	if len(s.Subscriptions) > 0 {
		parts := make([]string, 0, len(s.Subscriptions))
		for _, sub := range s.Subscriptions {
			p := sub.Name
			if st := strings.ToLower(strings.TrimSpace(sub.Status)); st != "" && st != "active" {
				p += " (" + st + ")"
			}
			if !sub.RenewsAt.IsZero() && now.Before(sub.RenewsAt) {
				p += " · renews " + sub.RenewsAt.Format("02 Jan 15:04") + " (in " + durationClock(sub.RenewsAt.Sub(now)) + ")"
			}
			parts = append(parts, p)
		}
		subsLine = strings.Join(parts, "   ·   ")
	}
	drawText(hdc, fontSmall, rgb(165, 170, 183), "Subscription", left, row, left+112, row+22, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontBody, rgb(231, 233, 238), subsLine, left+118, row, right, row+22, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	row += 24
	srvLine := "—"
	if !s.ServerTime.IsZero() {
		srvLine = "Server " + s.ServerTime.Format("15:04:05")
		if d := s.ServerDrift; d > 2*time.Second || d < -2*time.Second {
			srvLine += fmt.Sprintf(" (clock drift %s)", d.Round(time.Second))
		}
	}
	if s.DeviceMid != "" {
		mid := s.DeviceMid
		if len(mid) > 8 {
			mid = mid[:8]
		}
		if srvLine != "—" {
			srvLine += "   ·   "
		} else {
			srvLine = ""
		}
		srvLine += "mid " + mid
	}
	drawText(hdc, fontSmall, rgb(140, 145, 158), srvLine, left, row, right, row+22, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func drawStatPair(hdc uintptr, left, right int32, label, value string, y int32) {
	labelRight := left + 116
	drawText(hdc, fontSmall, rgb(165, 170, 183), label, left, y, labelRight, y+22, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontBody, rgb(235, 237, 242), value, labelRight+4, y, right, y+22, DT_RIGHT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func syncText(t time.Time) string {
	if t.IsZero() {
		return "not yet"
	}
	return t.Format("15:04:05")
}

func connectionFooter(s Snapshot) string {
	state := "Starting ZCode readers"
	if s.Connected && s.SignedIn {
		state = "Connected"
	} else if s.Connected {
		state = "Signed out"
	} else if s.Error != "" {
		state = "Disconnected"
	}
	footer := state + "   ·   Synced " + syncText(s.UpdatedAt) + "   ·   ZCode " + zcodeAppVersion
	if len(s.Balances) > 0 {
		names := make([]string, 0, len(s.Balances))
		for _, b := range s.Balances {
			names = append(names, b.ShowName)
		}
		footer += "   ·   " + strings.Join(names, ", ")
	}
	return footer
}

func drawBalanceCard(hdc uintptr, rc *RECT, b Balance, y int32, now time.Time) {
	const margin int32 = 22
	card := RECT{margin, y, rc.Right - margin, y + 134}
	fillPanel(hdc, card, rgb(22, 24, 29))
	framePanel(hdc, card, rgb(35, 38, 46))
	promo := !isRecurringPeriod(b.Period)
	if promo {
		// Blue accent marks the promotion pool, mirroring the pending
		// grant cards, so promo and regular quota never look alike.
		fillPanel(hdc, RECT{card.Left, card.Top, card.Left + 4, card.Bottom}, rgb(96, 165, 250))
	}

	used := balanceUsedPercent(b)
	remaining := remainingPct(b)

	title := strings.ToUpper(b.ShowName)
	if pl := periodLabel(b.Period); pl != "" {
		title += " · " + pl + " TOKENS"
	} else {
		title += " · TOKENS"
	}
	if promo {
		title += " · PROMO"
	}
	drawText(hdc, fontLabel, rgb(235, 237, 242), title, card.Left+14, y+8, card.Right-220, y+32, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	pct := fmt.Sprintf("%.0f%% used   ·   %.0f%% left", used, remaining)
	drawText(hdc, fontSmall, rgb(185, 190, 202), pct, card.Right-225, y+8, card.Right-14, y+32, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)

	bar := RECT{card.Left + 14, y + 36, card.Right - 14, y + 46}
	fillPanel(hdc, bar, rgb(43, 46, 55))
	fillW := int32(float64(bar.Right-bar.Left) * used / 100)
	if fillW > 0 {
		color := rgb(75, 170, 110)
		if used >= 90 {
			color = rgb(205, 75, 75)
		} else if used >= 75 {
			color = rgb(205, 157, 57)
		}
		fr := RECT{bar.Left, bar.Top, bar.Left + fillW, bar.Bottom}
		fillPanel(hdc, fr, color)
	}

	nums := fmt.Sprintf("%s used   ·   %s left   ·   %s total", formatInt64(b.Used), formatInt64(b.Remaining), formatInt64(b.Total))
	drawText(hdc, fontSmall, rgb(151, 157, 171), nums, card.Left+14, y+52, card.Right-14, y+72, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	countdown := "—"
	exact := "End time not reported"
	if !b.PeriodEnd.IsZero() {
		if now.Before(b.PeriodEnd) {
			countdown = durationClock(b.PeriodEnd.Sub(now))
		} else {
			countdown = "00:00:00"
		}
		if isRecurringPeriod(b.Period) {
			exact = "Refills: " + b.PeriodEnd.Format("Mon 02 Jan 2006 15:04:05") + " local"
		} else if now.Before(b.PeriodEnd) {
			// One-time promo pools expire rather than refill: whatever
			// is unused when the window closes is gone.
			exact = "EXPIRES: " + b.PeriodEnd.Format("Mon 02 Jan 2006 15:04:05") + " local — unused tokens are lost"
		} else {
			exact = "Expired " + b.PeriodEnd.Format("Mon 02 Jan 2006 15:04") + " — pool closed"
		}
	}
	countdownColor := rgb(249, 250, 252)
	if !isRecurringPeriod(b.Period) && !b.PeriodEnd.IsZero() {
		if now.Before(b.PeriodEnd) {
			countdownColor = rgb(242, 182, 94) // expiring pool: amber, not a refill
		} else {
			countdownColor = rgb(150, 150, 160)
		}
	}
	drawText(hdc, fontCountdownSmall, countdownColor, countdown, card.Left+14, y+68, card.Right-14, y+111, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fontSmall, rgb(154, 160, 174), exact, card.Left+14, y+110, card.Right-14, y+131, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

// modelNames strips the "model:" prefix from capabilities for display.
func modelNames(caps []string) string {
	var names []string
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		names = append(names, strings.TrimPrefix(c, "model:"))
	}
	return strings.Join(names, ", ")
}

func drawPendingGrantCard(hdc uintptr, rc *RECT, g PendingGrant, y int32, now time.Time) {
	const margin int32 = 22
	card := RECT{margin, y, rc.Right - margin, y + 134}
	fillPanel(hdc, card, rgb(22, 24, 29))
	framePanel(hdc, card, rgb(35, 38, 46))
	accent := RECT{card.Left, card.Top, card.Left + 4, card.Bottom}
	fillPanel(hdc, accent, rgb(96, 165, 250))

	title := strings.ToUpper(g.ShowName)
	if pl := periodLabel(g.Period); pl != "" {
		title += " · " + pl + " TOKENS"
	} else {
		title += " · TOKENS"
	}
	title += " · PROMO"
	drawText(hdc, fontLabel, rgb(235, 237, 242), title, card.Left+14, y+8, card.Right-230, y+32, DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontSmall, rgb(185, 190, 202), formatInt64(g.GrantUnits)+" TOKENS", card.Right-235, y+8, card.Right-14, y+32, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)

	nums := fmt.Sprintf("Grant: %s tokens", formatInt64(g.GrantUnits))
	if models := modelNames(g.Capabilities); models != "" {
		nums += "   ·   models: " + models
	}
	drawText(hdc, fontSmall, rgb(151, 157, 171), nums, card.Left+14, y+36, card.Right-14, y+56, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)

	countdown := "PENDING"
	exact := "Grant received — waiting for Z.AI to schedule it"
	if !g.EffectiveAt.IsZero() {
		if now.Before(g.EffectiveAt) {
			countdown = "STARTS IN " + durationClock(g.EffectiveAt.Sub(now))
			exact = "Activates: " + g.EffectiveAt.Format("Mon 02 Jan 2006 15:04:05") + " local"
		} else {
			countdown = "LIVE SOON"
			exact = "Activated at " + g.EffectiveAt.Format("15:04") + " — waiting for the bucket to appear"
		}
	}
	if !g.PlanEndsAt.IsZero() && now.Before(g.PlanEndsAt) {
		// The offer window is finite: unused tokens are lost when it closes.
		exact += "   ·   offer ends " + g.PlanEndsAt.Format("02 Jan 15:04") + " (in " + durationClock(g.PlanEndsAt.Sub(now)) + ")"
	} else if !g.PlanEndsAt.IsZero() {
		exact += "   ·   offer window closed " + g.PlanEndsAt.Format("02 Jan 15:04")
	}
	if g.PlanName != "" {
		exact += "   ·   " + g.PlanName
	}
	drawText(hdc, fontCountdownSmall, rgb(249, 250, 252), countdown, card.Left+14, y+54, card.Right-14, y+97, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fontSmall, rgb(154, 160, 174), exact, card.Left+14, y+96, card.Right-14, y+117, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
	drawText(hdc, fontSmall, rgb(125, 160, 220), "No usage yet — the pool appears above once it activates", card.Left+14, y+113, card.Right-14, y+131, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}

func durationClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	days := total / 86400
	total %= 86400
	h := total / 3600
	total %= 3600
	m := total / 60
	sec := total % 60
	if days > 0 {
		return fmt.Sprintf("%dd %02d:%02d:%02d", days, h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d:%02d", h, m, sec)
}

func publishSnapshot(s Snapshot) { dataMu.Lock(); currentSnapshot = s; dataMu.Unlock(); postRefresh() }
func postRefresh() {
	if hwndMain == 0 {
		return
	}
	if atomic.CompareAndSwapInt32(&refreshPosted, 0, 1) {
		procPostMessageW.Call(hwndMain, WM_APP_REFRESH, 0, 0)
	}
}
func cloneSnapshot(s Snapshot) Snapshot {
	c := s
	c.Plans = append([]Plan(nil), s.Plans...)
	c.Balances = append([]Balance(nil), s.Balances...)
	if s.Customer != nil {
		v := *s.Customer
		c.Customer = &v
	}
	return c
}

func resizeForSnapshot() {
	dataMu.RLock()
	s := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()

	clientH := int32(300)
	if s.Connected && s.SignedIn {
		clientH = 76 + 94 + 30
		if len(s.Balances) == 0 {
			clientH += 82
		} else {
			clientH += int32(len(s.Balances) * 146)
		}
		if pend := pendingGrants(s, time.Now()); len(pend) > 0 {
			clientH += 6 + 30 + int32(len(pend)*146)
		}
		if extra := len(s.Plans) - 1; extra > 0 {
			clientH += int32(extra) * 24
		}
		clientH += 204 + 24 // +24: subscription line
		clientH += 32
		if clientH < 600 {
			clientH = 600
		}
	} else if s.Connected && !s.SignedIn {
		clientH = 78 + 196 + 42
	}

	var wa RECT
	procSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	maxH := wa.Bottom - wa.Top - 16
	if maxH > 0 && clientH > maxH {
		clientH = maxH
	}
	if clientH < 300 {
		clientH = 300
	}
	changed := clientH != winHeight
	winHeight = clientH
	if collapsed || !changed {
		return
	}
	if snapped {
		snapToCorner()
	} else {
		procSetWindowPos.Call(hwndMain, ^uintptr(0), 0, 0, uintptr(winWidth), uintptr(winHeight), SWP_NOMOVE|SWP_SHOWWINDOW)
	}
}

func taskbarRects() (RECT, RECT, bool) {
	var taskbar, notify RECT
	cls, _ := syscall.UTF16PtrFromString("Shell_TrayWnd")
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return taskbar, notify, false
	}
	if r, _, _ := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&taskbar))); r == 0 {
		return taskbar, notify, false
	}
	childCls, _ := syscall.UTF16PtrFromString("TrayNotifyWnd")
	child, _, _ := procFindWindowExW.Call(h, 0, uintptr(unsafe.Pointer(childCls)), 0)
	if child != 0 {
		procGetWindowRect.Call(child, uintptr(unsafe.Pointer(&notify)))
	}
	return taskbar, notify, true
}

// collapsedPreviewRowCount counts the rows the collapsed panel lists:
// one per real bucket (total > 0) plus one per pending promotion grant,
// capped at four plus a "+N more" row. Zero means there is nothing to
// list (signed out, connecting, no buckets) and the panel falls back to
// the taskbar-sized strip.
func collapsedPreviewRowCount(s Snapshot) int32 {
	if !s.Connected || !s.SignedIn {
		return 0
	}
	n := int32(0)
	for _, b := range s.Balances {
		if b.Total > 0 {
			n++
		}
	}
	n += int32(len(pendingGrants(s, time.Now())))
	if n == 0 {
		return 0
	}
	if n > 4 {
		return 5
	}
	return n
}

// collapsedPanelHeight sizes the collapsed panel: a taskbar-sized strip
// when there is nothing to list, otherwise one full-width row per
// bucket — stacked rows give each pool far more room than side-by-side
// segments. Pure function — unit-testable.
func collapsedPanelHeight(rows, taskbarH int32) int32 {
	if rows <= 0 {
		if taskbarH >= 30 && taskbarH <= 100 {
			return taskbarH
		}
		return 40
	}
	const btnStrip, rowH, gap, pad int32 = 24, 34, 4, 8
	return btnStrip + rows*rowH + (rows-1)*gap + pad
}

func collapsedGeometry() (x, y, w, h int32) {
	x, y, w, h, _ = collapsedGeometryEx()
	return
}

func collapsedGeometryEx() (x, y, w, h int32, companions []RECT) {
	var wa RECT
	procSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	w = 240
	if _, notify, ok := taskbarRects(); ok {
		nw := notify.Right - notify.Left
		if nw > 0 {
			w = (nw * 2) / 3
		}
	}
	if w < 200 {
		w = 200
	}
	if w > 340 {
		w = 340
	}
	// Height follows the content: one stacked row per bucket, or the
	// taskbar-sized strip when there is nothing to list.
	dataMu.RLock()
	snap := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()
	var taskbarH int32 = 40
	if taskbar, _, ok := taskbarRects(); ok {
		th := taskbar.Bottom - taskbar.Top
		if th >= 30 && th <= 100 {
			taskbarH = th
		}
	}
	h = collapsedPanelHeight(collapsedPreviewRowCount(snap), taskbarH)
	x = wa.Right - w
	y = wa.Bottom - h
	// The installed Codex HUD parks its own collapsed strip on this exact
	// rectangle. Stack above any visible companion window instead of
	// covering it.
	companions = visibleCompanionRects()
	base := RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
	stacked := stackAboveRect(base, companions, companionStackGap, wa.Top)
	return stacked.Left, stacked.Top, stacked.Right - stacked.Left, stacked.Bottom - stacked.Top, companions
}

// visibleCompanionRects returns the screen rects of visible companion HUD
// windows (Codex HUD builds). Hidden or degenerate windows are ignored.
func visibleCompanionRects() []RECT {
	var out []RECT
	for _, class := range companionHUDClasses {
		cls, _ := syscall.UTF16PtrFromString(class)
		if cls == nil {
			continue
		}
		h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
		if h == 0 {
			continue
		}
		vis, _, _ := procIsWindowVisible.Call(h)
		if vis == 0 {
			continue
		}
		var r RECT
		if rr, _, _ := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); rr == 0 {
			continue
		}
		if r.Right <= r.Left || r.Bottom <= r.Top {
			continue
		}
		out = append(out, r)
	}
	return out
}

// stackAboveRect moves base straight up until it no longer overlaps any
// obstacle it horizontally overlaps, keeping a gap. Touching edges count
// as non-overlapping so the result is stable across repeated calls.
// Pure geometry (no Win32 calls) so it stays unit-testable.
func stackAboveRect(base RECT, obstacles []RECT, gap, minY int32) RECT {
	r := base
	h := r.Bottom - r.Top
	if h <= 0 {
		return base
	}
	for iter := 0; iter <= len(obstacles); iter++ {
		moved := false
		for _, o := range obstacles {
			if r.Right <= o.Left || r.Left >= o.Right {
				continue
			}
			if r.Bottom <= o.Top || r.Top >= o.Bottom {
				continue
			}
			r.Top = o.Top - h - gap
			r.Bottom = o.Top - gap
			moved = true
		}
		if !moved {
			break
		}
		if r.Top < minY {
			break
		}
	}
	if r.Top < minY {
		r.Top = minY
		r.Bottom = minY + h
	}
	return r
}

// restackCollapsed re-applies the stacked collapsed position. It runs on
// the 1-second timer so the strip follows the companion HUD when the
// companion appears, moves, expands, collapses, or exits. SetWindowPos is
// only issued when the rect actually changed.
func restackCollapsed() {
	if hwndMain == 0 || !collapsed {
		return
	}
	x, y, w, h, companions := collapsedGeometryEx()
	var cur RECT
	if rr, _, _ := procGetWindowRect.Call(hwndMain, uintptr(unsafe.Pointer(&cur))); rr != 0 {
		if cur.Left == x && cur.Top == y && cur.Right-cur.Left == w && cur.Bottom-cur.Top == h {
			return
		}
	}
	procSetWindowPos.Call(hwndMain, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_SHOWWINDOW)
	logStackPlacement("restack", x, y, w, h, companions)
}

// restackExpanded re-applies the snapped expanded position so a snapped
// panel follows the companion HUD when it appears, moves, or exits. It
// never runs while the user is dragging the panel (snapped=false) and
// only issues SetWindowPos when the rect actually changed.
func restackExpanded() {
	if hwndMain == 0 || collapsed || !snapped {
		return
	}
	var wa RECT
	procSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	x := wa.Right - winWidth - 10
	y := wa.Bottom - winHeight - 10
	base := RECT{Left: x, Top: y, Right: x + winWidth, Bottom: y + winHeight}
	companions := visibleCompanionRects()
	stacked := stackAboveRect(base, companions, companionStackGap, wa.Top)
	var cur RECT
	if rr, _, _ := procGetWindowRect.Call(hwndMain, uintptr(unsafe.Pointer(&cur))); rr != 0 {
		if cur.Left == stacked.Left && cur.Top == stacked.Top && cur.Right-cur.Left == winWidth && cur.Bottom-cur.Top == winHeight {
			return
		}
	}
	procSetWindowPos.Call(hwndMain, ^uintptr(0), uintptr(stacked.Left), uintptr(stacked.Top), uintptr(winWidth), uintptr(winHeight), SWP_SHOWWINDOW)
	logStackPlacement("restack-expanded", stacked.Left, stacked.Top, winWidth, winHeight, companions)
}

func positionCollapsed() {
	if hwndMain == 0 {
		return
	}
	x, y, w, h, companions := collapsedGeometryEx()
	procSetWindowPos.Call(hwndMain, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), SWP_SHOWWINDOW)
	logStackPlacement("collapse", x, y, w, h, companions)
}

func logStackPlacement(why string, x, y, w, h int32, companions []RECT) {
	desc := "none"
	if len(companions) > 0 {
		parts := make([]string, 0, len(companions))
		for _, r := range companions {
			parts = append(parts, fmt.Sprintf("[%d,%d %dx%d]", r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top))
		}
		desc = strings.Join(parts, " ")
	}
	logDiagnostic("stack %s: strip=%d,%d %dx%d companions=%d %s", why, x, y, w, h, len(companions), desc)
}

func snapToCorner() {
	if hwndMain == 0 {
		return
	}
	if collapsed {
		positionCollapsed()
		return
	}
	var wa RECT
	procSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(&wa)), 0)
	x := wa.Right - winWidth - 10
	y := wa.Bottom - winHeight - 10
	// Expanded mode stacks too: the Codex HUD (strip or full panel) sits
	// in the same notification-area corner, and covering it hides the
	// other tool entirely.
	base := RECT{Left: x, Top: y, Right: x + winWidth, Bottom: y + winHeight}
	companions := visibleCompanionRects()
	stacked := stackAboveRect(base, companions, companionStackGap, wa.Top)
	procSetWindowPos.Call(hwndMain, ^uintptr(0), uintptr(stacked.Left), uintptr(stacked.Top), uintptr(stacked.Right-stacked.Left), uintptr(stacked.Bottom-stacked.Top), SWP_SHOWWINDOW)
	logStackPlacement("snap-expanded", stacked.Left, stacked.Top, stacked.Right-stacked.Left, stacked.Bottom-stacked.Top, companions)
}

func showMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	showText := "Show HUD"
	if isHUDVisible() {
		if collapsed {
			showText = "Expand HUD"
		} else {
			showText = "Collapse HUD"
		}
	}
	appendMenu(menu, MF_STRING, ID_SHOWHIDE, showText)
	appendMenu(menu, MF_STRING, ID_REFRESH, "Refresh now")
	appendMenu(menu, MF_STRING, ID_SNAP, "Snap to notification-area corner")
	appendMenu(menu, MF_SEPARATOR, 0, "")
	dataMu.RLock()
	signedIn := currentSnapshot.SignedIn
	loginWait := currentSnapshot.LoginPending
	dataMu.RUnlock()
	if loginWait {
		appendMenu(menu, MF_STRING, ID_SIGNIN, "Cancel sign-in")
	} else if signedIn {
		appendMenu(menu, MF_STRING, ID_LOGOUT, "Sign out")
	} else {
		appendMenu(menu, MF_STRING, ID_SIGNIN, "Sign in with Google…")
		appendMenu(menu, MF_STRING, ID_IMPORT, "Use ZCode app's session")
	}
	appendMenu(menu, MF_STRING, ID_OPENZ, "Open ZCode folder")
	startupText := "Start with Windows (compact)"
	if startupEnabled() {
		startupText = "Stop starting with Windows"
	}
	appendMenu(menu, MF_STRING, ID_STARTUP, startupText)
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING, ID_EXIT, "Exit ZCode Usage HUD")
	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, TPM_RIGHTBUTTON|TPM_RETURNCMD, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procPostMessageW.Call(hwnd, WM_NULL, 0, 0)
	if cmd != 0 {
		handleCommand(hwnd, int(cmd))
	}
}

func handleCommand(hwnd uintptr, id int) {
	switch id {
	case ID_REFRESH:
		go refreshNow()
	case ID_SNAP:
		snapped = true
		snapToCorner()
	case ID_STARTUP:
		enable := !startupEnabled()
		if err := setStartupPath(enable, installedExe()); err != nil {
			logDiagnostic("startup toggle failed enable=%t: %v", enable, err)
			messageBox("Start with Windows", "Windows could not update the startup setting.\n\n"+err.Error()+"\n\nDetails were written to:\n"+diagnosticLogPath())
		} else if enable {
			logDiagnostic("startup enabled and verified")
			showTrayNotification("Start with Windows enabled", "ZCode Usage HUD will open in compact mode at your next sign-in.")
		} else {
			logDiagnostic("startup disabled and verified")
			showTrayNotification("Start with Windows disabled", "ZCode Usage HUD will no longer open automatically at sign-in.")
		}
	case ID_OPENZ:
		shellOpen(zcodeDir())
	case ID_SIGNIN:
		toggleGoogleLogin()
	case ID_IMPORT:
		go importZCodeAppSession()
	case ID_LOGOUT:
		go signOut()
	case ID_SHOWHIDE:
		toggleHUD()
	case ID_EXIT:
		procDestroyWindow.Call(hwnd)
	}
}
func appendMenu(menu uintptr, flags uint32, id int, text string) {
	var p uintptr
	if text != "" {
		u, _ := syscall.UTF16PtrFromString(text)
		p = uintptr(unsafe.Pointer(u))
	}
	procAppendMenuW.Call(menu, uintptr(flags), uintptr(id), p)
}

func mouseXY(lParam uintptr) (int32, int32) {
	x := int32(int16(uint16(lParam & 0xffff)))
	y := int32(int16(uint16((lParam >> 16) & 0xffff)))
	return x, y
}

func pointInRect(x, y int32, rc RECT) bool {
	return x >= rc.Left && x < rc.Right && y >= rc.Top && y < rc.Bottom
}

func titleButtonRectsFromClient(rc RECT) (RECT, RECT) {
	h := int32(43)
	bw := int32(46)
	if collapsed {
		// Slim strip at the very top so the bucket rows below can use
		// the full panel width for their bars.
		h = 18
		bw = 36
	}
	closeRc := RECT{rc.Right - bw, 0, rc.Right, h}
	minRc := RECT{rc.Right - 2*bw, 0, rc.Right - bw, h}
	return minRc, closeRc
}

func titleButtonRects(hwnd uintptr) (RECT, RECT) {
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	return titleButtonRectsFromClient(rc)
}

func invalidateTitleBar(hwnd uintptr) {
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	rc.Bottom = 45
	procInvalidateRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)), 0)
}

func snapshotNeedsOneSecondPaint() bool {
	dataMu.RLock()
	s := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()
	if !s.Connected || !s.SignedIn {
		return false
	}
	now := time.Now()
	for _, b := range s.Balances {
		if !b.PeriodEnd.IsZero() && now.Before(b.PeriodEnd) {
			return true
		}
	}
	for _, g := range pendingGrants(s, now) {
		if !g.EffectiveAt.IsZero() && now.Before(g.EffectiveAt) {
			return true
		}
	}
	if len(s.Plans) > 0 && !s.Plans[0].EndsAt.IsZero() && now.Before(s.Plans[0].EndsAt) {
		return true
	}
	return false
}

func invalidateDynamicRegions(hwnd uintptr) {
	procInvalidateRect.Call(hwnd, 0, 0)
}

func createAppIcon() uintptr {
	const w = 32
	const h = 32
	andMask := make([]byte, h*4)
	xorBits := make([]byte, w*h*4)
	cx, cy := 15.5, 15.5
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			d2 := dx*dx + dy*dy
			inside := d2 <= 14.5*14.5
			if !inside {
				idx := y*4 + x/8
				andMask[idx] |= byte(1 << uint(7-(x%8)))
				continue
			}
			r, g, b := byte(24), byte(24), byte(30)
			ring := d2 >= 10.5*10.5 && d2 <= 14.5*14.5
			opening := x >= 20 && y >= 10 && y <= 21
			if ring && !opening {
				r, g, b = 86, 204, 132
			}
			if x >= 10 && x <= 18 && y >= 11 && y <= 20 && absInt((x-10)-(20-y)) <= 1 {
				r, g, b = 130, 225, 162
			}
			row := h - 1 - y
			i := (row*w + x) * 4
			xorBits[i+0] = b
			xorBits[i+1] = g
			xorBits[i+2] = r
			xorBits[i+3] = 255
		}
	}
	r, _, _ := procCreateIcon.Call(0, w, h, 1, 32,
		uintptr(unsafe.Pointer(&andMask[0])), uintptr(unsafe.Pointer(&xorBits[0])))
	return r
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func createFont(points, weight int32) uintptr {
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	r, _, _ := procCreateFontW.Call(uintptr(int32(-points-4)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return r
}
func createBrush(c uint32) uintptr { r, _, _ := procCreateSolidBrush.Call(uintptr(c)); return r }
func rgb(r, g, b byte) uint32      { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }
func drawText(hdc, font uintptr, color uint32, text string, l, t, r, b int32, flags uint32) {
	if text == "" {
		return
	}
	old, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, old)
	procSetTextColor.Call(hdc, uintptr(color))
	u, _ := syscall.UTF16FromString(text)
	rc := RECT{l, t, r, b}
	procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&rc)), uintptr(flags))
}

func formatInt64(n int64) string {
	str := strconv.FormatInt(n, 10)
	neg := ""
	if strings.HasPrefix(str, "-") {
		neg = "-"
		str = str[1:]
	}
	for i := len(str) - 3; i > 0; i -= 3 {
		str = str[:i] + "," + str[i:]
	}
	return neg + str
}

func shellOpen(url string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	u, _ := syscall.UTF16PtrFromString(url)
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(u)), 0, 0, SW_SHOWNORMAL)
}

// toggleGoogleLogin starts the browser sign-in, or cancels a pending one.
func toggleGoogleLogin() {
	loginMu.Lock()
	pending := loginPending
	loginMu.Unlock()
	if pending {
		cancelGoogleLogin()
		return
	}
	dataMu.RLock()
	signedIn := currentSnapshot.SignedIn
	dataMu.RUnlock()
	if signedIn {
		return
	}
	go startGoogleLogin()
}

// credsModTime reports the mtime of the HUD's credential store so the
// HUD notices a fresh sign-in without waiting for the 60s poll.
func credsModTime() time.Time {
	fi, err := os.Stat(hudCredsPath())
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// storedDeviceMid reads the device id ZCode uses in API headers.
func storedDeviceMid() string {
	raw, err := os.ReadFile(filepath.Join(zcodeDir(), "telemetry-state.json"))
	if err != nil {
		return ""
	}
	var t struct {
		DeviceMid string `json:"deviceMid"`
	}
	if json.Unmarshal(raw, &t) != nil {
		return ""
	}
	return t.DeviceMid
}

const (
	startupValueName   = "ZCodeUsageHUD"
	startupRunKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupApprovalKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

func startupCommand(exe string) string {
	return `"` + filepath.Clean(exe) + `" --hud --collapsed --startup`
}

func startupRegistered(exe string) (bool, error) {
	key, err := openRegKey(startupRunKey, KEY_QUERY_VALUE)
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf(": %d", ERROR_FILE_NOT_FOUND)) {
			return false, nil
		}
		return false, err
	}
	defer procRegCloseKey.Call(key)
	value, exists, err := regQueryString(key, startupValueName)
	if err != nil || !exists {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(value), startupCommand(exe)), nil
}

func startupDisabledByWindows() bool {
	key, err := openRegKey(startupApprovalKey, KEY_QUERY_VALUE)
	if err != nil {
		return false
	}
	defer procRegCloseKey.Call(key)
	data, exists, err := regQueryBytes(key, startupValueName)
	return err == nil && exists && len(data) > 0 && data[0] == 3
}

func startupEnabled() bool {
	registered, err := startupRegistered(installedExe())
	return err == nil && registered && !startupDisabledByWindows()
}

func setStartupPath(enable bool, exe string) error {
	key, err := createRegKey(startupRunKey, KEY_SET_VALUE)
	if err != nil {
		return err
	}
	if !enable {
		err := regDeleteValue(key, startupValueName)
		procRegCloseKey.Call(key)
		if err != nil {
			return err
		}
		_ = clearStartupApproval()
		registered, verifyErr := startupRegistered(exe)
		if verifyErr != nil {
			return verifyErr
		}
		if registered {
			return fmt.Errorf("Windows retained the startup entry after it was disabled")
		}
		return nil
	}
	if err := regSetString(key, startupValueName, startupCommand(exe)); err != nil {
		procRegCloseKey.Call(key)
		return err
	}
	procRegCloseKey.Call(key)
	if err := clearStartupApproval(); err != nil {
		return fmt.Errorf("could not clear Windows' disabled startup state: %w", err)
	}
	registered, err := startupRegistered(exe)
	if err != nil {
		return err
	}
	if !registered || startupDisabledByWindows() {
		return fmt.Errorf("Windows did not retain the verified startup entry")
	}
	return nil
}

func clearStartupApproval() error {
	key, err := openRegKey(startupApprovalKey, KEY_SET_VALUE)
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf(": %d", ERROR_FILE_NOT_FOUND)) {
			return nil
		}
		return err
	}
	defer procRegCloseKey.Call(key)
	return regDeleteValue(key, startupValueName)
}

func openRegKey(sub string, access uint32) (uintptr, error) {
	p, _ := syscall.UTF16PtrFromString(sub)
	var key uintptr
	r, _, _ := procRegOpenKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(p)), 0, uintptr(access), uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return 0, fmt.Errorf("RegOpenKeyExW: %d", r)
	}
	return key, nil
}

func regQueryString(key uintptr, name string) (string, bool, error) {
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	r, _, _ := procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	if r == ERROR_FILE_NOT_FOUND {
		return "", false, nil
	}
	if r != 0 {
		return "", false, fmt.Errorf("RegQueryValueExW(%s): %d", name, r)
	}
	if typ != REG_SZ || size < 2 {
		return "", true, fmt.Errorf("registry value %s is not a non-empty REG_SZ", name)
	}
	data := make([]uint16, (size+1)/2)
	r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return "", false, fmt.Errorf("RegQueryValueExW(%s): %d", name, r)
	}
	return syscall.UTF16ToString(data), true, nil
}

func regQueryBytes(key uintptr, name string) ([]byte, bool, error) {
	n, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	r, _, _ := procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	if r == ERROR_FILE_NOT_FOUND {
		return nil, false, nil
	}
	if r != 0 {
		return nil, false, fmt.Errorf("RegQueryValueExW(%s): %d", name, r)
	}
	if typ != REG_BINARY || size == 0 {
		return nil, true, nil
	}
	data := make([]byte, size)
	r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return nil, false, fmt.Errorf("RegQueryValueExW(%s): %d", name, r)
	}
	return data, true, nil
}

func regDeleteValue(key uintptr, name string) error {
	n, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := procRegDeleteValueW.Call(key, uintptr(unsafe.Pointer(n)))
	if r != 0 && r != ERROR_FILE_NOT_FOUND {
		return fmt.Errorf("RegDeleteValueW(%s): %d", name, r)
	}
	return nil
}

func createRegKey(sub string, access uint32) (uintptr, error) {
	p, _ := syscall.UTF16PtrFromString(sub)
	var key uintptr
	r, _, _ := procRegCreateKeyExW.Call(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(p)), 0, 0, 0, uintptr(access), 0, uintptr(unsafe.Pointer(&key)), 0)
	if r != 0 {
		return 0, fmt.Errorf("RegCreateKeyExW: %d", r)
	}
	return key, nil
}
func regSetString(key uintptr, name, value string) error {
	n, _ := syscall.UTF16PtrFromString(name)
	d, _ := syscall.UTF16FromString(value)
	r, _, _ := procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, REG_SZ, uintptr(unsafe.Pointer(&d[0])), uintptr(len(d)*2))
	if r != 0 {
		return fmt.Errorf("RegSetValueExW: %d", r)
	}
	return nil
}
func regSetDWORD(key uintptr, name string, value uint32) error {
	n, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(n)), 0, REG_DWORD, uintptr(unsafe.Pointer(&value)), 4)
	if r != 0 {
		return fmt.Errorf("RegSetValueExW: %d", r)
	}
	return nil
}
