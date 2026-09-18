//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBalanceUsedPercent(t *testing.T) {
	b := Balance{Total: 3000000, Used: 1500000}
	if got := balanceUsedPercent(b); got != 50 {
		t.Fatalf("got %v", got)
	}
	if got := balanceUsedPercent(Balance{}); got != 0 {
		t.Fatalf("empty got %v", got)
	}
	if got := balanceUsedPercent(Balance{Total: 100, Used: 200}); got != 100 {
		t.Fatalf("clamp got %v", got)
	}
}

func TestComputeUnlock(t *testing.T) {
	now := time.Now()
	refill := now.Add(3 * time.Hour)
	bs := []Balance{
		{ShowName: "a", Total: 100, Used: 100, Remaining: 0, PeriodEnd: refill},
		{ShowName: "b", Total: 100, Used: 100, Remaining: 0, PeriodEnd: refill.Add(time.Hour)},
	}
	unlock, locked := computeUnlock(bs, now)
	if !locked {
		t.Fatal("expected locked")
	}
	if !unlock.Equal(refill.Add(time.Hour)) {
		t.Fatalf("unlock %v", unlock)
	}
	bs[0].Remaining = 5
	if _, locked := computeUnlock(bs, now); locked {
		t.Fatal("expected available")
	}
	if _, locked := computeUnlock(nil, now); locked {
		t.Fatal("empty should not lock")
	}
}

func TestComputeUnlockIgnoresZeroTotalBuckets(t *testing.T) {
	now := time.Now()
	// A zero-total placeholder bucket carries no quota data: it must
	// neither lock the HUD nor keep it "available".
	if unlock, locked := computeUnlock([]Balance{{Total: 0, Remaining: 0, PeriodEnd: now.Add(time.Hour)}}, now); locked || !unlock.IsZero() {
		t.Fatalf("zero-total bucket must not lock: unlock %v locked %v", unlock, locked)
	}
	// Mixed: placeholder + exhausted real bucket → locked with refill.
	mixed := []Balance{
		{Total: 0, Remaining: 0},
		{Period: "daily", Total: 100, Used: 100, Remaining: 0, PeriodEnd: now.Add(2 * time.Hour)},
	}
	if unlock, locked := computeUnlock(mixed, now); !locked || unlock.IsZero() {
		t.Fatalf("want locked with refill, got unlock %v locked %v", unlock, locked)
	}
}

func TestAggregateRemainingPct(t *testing.T) {
	// One exhausted daily bucket must not drag the account-wide number
	// to zero while a big promo pool is still full.
	bs := []Balance{
		{ShowName: "GLM-5.3", Total: 3000000, Remaining: 0},
		{ShowName: "GLM-5.3-Flash", Period: "one_time", Total: 100000000, Remaining: 96000000},
		{ShowName: "GLM-5.3-Flash", Total: 5000000, Remaining: 5000000},
	}
	if got := aggregateRemainingPct(bs); got < 93 || got > 94 {
		t.Fatalf("aggregate %v, want ~93.5", got)
	}
	if got := aggregateRemainingPct(nil); got != 100 {
		t.Fatalf("empty %v", got)
	}
	if got := aggregateRemainingPct([]Balance{{Total: 0}}); got != 100 {
		t.Fatalf("zero-total-only %v", got)
	}
	allEmpty := []Balance{{Total: 100, Remaining: 0}, {Total: 50, Remaining: 0}}
	if got := aggregateRemainingPct(allEmpty); got != 0 {
		t.Fatalf("all exhausted %v", got)
	}
}

func TestBucketQualifier(t *testing.T) {
	qual, color := bucketQualifier(Balance{ShowName: "GLM-5.3-Flash", Period: "one_time"})
	if qual != "PROMO" || color == 0 {
		t.Fatalf("one_time qualifier %q", qual)
	}
	// Recurring periods name their cycle, not "PROMO".
	for period, want := range map[string]string{
		"daily": "DAILY", "weekly": "WEEKLY", "monthly": "MONTHLY",
	} {
		if qual, _ := bucketQualifier(Balance{Period: period}); qual != want {
			t.Fatalf("period %q qualifier %q, want %q", period, qual, want)
		}
	}
	// Unknown recurring periods still never read as promo.
	if qual, _ := bucketQualifier(Balance{Period: "mystery"}); qual != "QUOTA" {
		t.Fatalf("mystery qualifier %q", qual)
	}
}

func TestCollapsedPreviewRowCount(t *testing.T) {
	s := Snapshot{Connected: true, SignedIn: true, Balances: []Balance{
		{Total: 100}, {Total: 50}, {Total: 0}, // zero-total rows are not listed
	}}
	if got := collapsedPreviewRowCount(s); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := collapsedPreviewRowCount(Snapshot{Connected: true}); got != 0 {
		t.Fatalf("signed-out %d", got)
	}
	many := Snapshot{Connected: true, SignedIn: true, Balances: []Balance{{Total: 1}, {Total: 1}, {Total: 1}, {Total: 1}, {Total: 1}, {Total: 1}}}
	if got := collapsedPreviewRowCount(many); got != 5 { // 4 buckets + "+N more"
		t.Fatalf("many %d", got)
	}
}

func TestCollapsedPanelHeight(t *testing.T) {
	if got := collapsedPanelHeight(0, 46); got != 46 {
		t.Fatalf("taskbar strip %d", got)
	}
	if got := collapsedPanelHeight(0, 500); got != 40 {
		t.Fatalf("bogus taskbar clamps to 40, got %d", got)
	}
	// 24 button strip + 3*34 rows + 2*4 gaps + 8 pad = 142.
	if got := collapsedPanelHeight(3, 46); got != 142 {
		t.Fatalf("stacked rows %d", got)
	}
}

func TestParseSubscriptions(t *testing.T) {
	// A populated list with common field spellings.
	body := []byte(`{"code":200,"success":true,"data":[` +
		`{"planName":"GLM Coding Plan","status":"active","expireTime":1893456000},` +
		`{"name":"Another","status":"expired"}]}`)
	subs := parseSubscriptions(body)
	if len(subs) != 2 {
		t.Fatalf("got %d subs: %+v", len(subs), subs)
	}
	if subs[0].Name != "GLM Coding Plan" || subs[0].Status != "active" || subs[0].RenewsAt.IsZero() {
		t.Fatalf("sub0 %+v", subs[0])
	}
	if subs[1].Name != "Another" || subs[1].Status != "expired" {
		t.Fatalf("sub1 %+v", subs[1])
	}
	// Failure envelopes produce nil.
	if got := parseSubscriptions([]byte(`{"code":500,"success":false,"data":[]}`)); got != nil {
		t.Fatalf("failure envelope: %+v", got)
	}
	// Empty list is an empty (non-nil-handled) slice.
	if got := parseSubscriptions([]byte(`{"code":200,"success":true,"data":[]}`)); len(got) != 0 {
		t.Fatalf("empty list: %+v", got)
	}
	// Items without any recognizable name are skipped.
	if got := parseSubscriptions([]byte(`{"code":200,"success":true,"data":[{"foo":"bar"}]}`)); len(got) != 0 {
		t.Fatalf("name-less item: %+v", got)
	}
}

func TestExpiringPromos(t *testing.T) {
	now := time.Now()
	bs := []Balance{
		// One-time pool deep in its final 24h → warned.
		{BucketID: "b1", ShowName: "GLM-5.3-Flash", Period: "one_time", Remaining: 1000, PeriodEnd: now.Add(7 * time.Hour)},
		// One-time pool far from expiry → skipped.
		{BucketID: "b2", Period: "one_time", Remaining: 1000, PeriodEnd: now.Add(48 * time.Hour)},
		// One-time pool already expired → skipped.
		{BucketID: "b3", Period: "one_time", Remaining: 1000, PeriodEnd: now.Add(-time.Hour)},
		// One-time pool fully spent → skipped.
		{BucketID: "b4", Period: "one_time", Remaining: 0, PeriodEnd: now.Add(2 * time.Hour)},
		// Recurring bucket → never warned (it refills).
		{BucketID: "b5", Period: "daily", Remaining: 0, PeriodEnd: now.Add(time.Hour)},
	}
	got := expiringPromos(bs, now, 24*time.Hour)
	if len(got) != 1 || got[0].BucketID != "b1" {
		t.Fatalf("got %+v", got)
	}
}

func TestCollapsedPreviewRowCountPending(t *testing.T) {
	now := time.Now()
	// Two live buckets + one pending promo = 3 rows.
	s := Snapshot{Connected: true, SignedIn: true,
		Balances: []Balance{{Total: 100, Remaining: 50}, {Total: 100, Remaining: 50}},
		Plans: []Plan{{EndsAt: now.Add(time.Hour), Entitlements: []Entitlement{
			{EntitlementID: "e1", GrantUnits: 5},
		}}}}
	if got := collapsedPreviewRowCount(s); got != 3 {
		t.Fatalf("got %d", got)
	}
}

func TestCreateShellLink(t *testing.T) {
	// COM smoke test: a .lnk must be created on disk and be plausibly
	// sized (no UI, no registry changes).
	path := t.TempDir() + "\\nested\\ZCode Usage HUD.lnk"
	if err := createShellLink(path, "C:\\Windows\\System32\\cmd.exe", "--hud", "test shortcut", ""); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() < 100 {
		t.Fatalf("shortcut suspiciously small: %d bytes", fi.Size())
	}
}

func TestIsRecurringPeriod(t *testing.T) {
	for _, p := range []string{"daily", "weekly", "monthly", "yearly", "annual", "", "mystery"} {
		if !isRecurringPeriod(p) {
			t.Fatalf("period %q should be recurring", p)
		}
	}
	for _, p := range []string{"one_time", "ONE-TIME", "onetime", "lifetime"} {
		if isRecurringPeriod(p) {
			t.Fatalf("period %q should not be recurring", p)
		}
	}
}

// A one-time promo pool must never pose as a refill source: not for the
// unlock countdown when exhausted, and not for the next-refill line.
func TestOneTimeBucketsAreNotRefills(t *testing.T) {
	now := time.Now()
	promoEnd := now.Add(8 * time.Hour)
	dailyEnd := now.Add(23 * time.Hour)
	bs := []Balance{
		{ShowName: "promo", Period: "one_time", Total: 100000000, Remaining: 96000000, PeriodEnd: promoEnd},
		{ShowName: "daily", Period: "daily", Total: 3000000, Remaining: 0, PeriodEnd: dailyEnd},
	}
	if got := earliestRefill(bs); !got.Equal(dailyEnd) {
		t.Fatalf("earliestRefill %v, want daily %v (promo expiry leaked in)", got, dailyEnd)
	}
	// Only the promo left, fully used: locked with no refill time.
	if unlock, locked := computeUnlock([]Balance{{Period: "one_time", Total: 100, Used: 100, Remaining: 0, PeriodEnd: promoEnd}}, now); !locked || !unlock.IsZero() {
		t.Fatalf("unlock %v locked %v; want locked with no refill time", unlock, locked)
	}
	// Mixed exhausted buckets: unlock counts only recurring ones.
	mixed := []Balance{
		{Period: "one_time", Total: 100, Used: 100, Remaining: 0, PeriodEnd: promoEnd},
		{Period: "daily", Total: 100, Used: 100, Remaining: 0, PeriodEnd: dailyEnd},
	}
	if unlock, locked := computeUnlock(mixed, now); !locked || !unlock.Equal(dailyEnd) {
		t.Fatalf("unlock %v locked %v, want daily-end", unlock, locked)
	}
}

func TestPendingGrantsSkipExpiredPlans(t *testing.T) {
	now := time.Now()
	s := Snapshot{Plans: []Plan{
		{Name: "live promo", UserPlanID: "u1", EndsAt: now.Add(time.Hour),
			Entitlements: []Entitlement{{EntitlementID: "e1", GrantUnits: 5}}},
		{Name: "dead promo", UserPlanID: "u2", EndsAt: now.Add(-time.Hour),
			Entitlements: []Entitlement{{EntitlementID: "e2", GrantUnits: 5}}},
	}}
	pend := pendingGrants(s, now)
	if len(pend) != 1 || pend[0].PlanName != "live promo" {
		t.Fatalf("got %+v", pend)
	}
	if pend[0].PlanEndsAt.IsZero() || !pend[0].PlanEndsAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("PlanEndsAt not carried: %+v", pend[0])
	}
}

func TestDurationClock(t *testing.T) {
	if got := durationClock(90 * time.Minute); !strings.Contains(got, "01:30:00") {
		t.Fatal(got)
	}
	if got := durationClock(26 * time.Hour); !strings.Contains(got, "1d") {
		t.Fatal(got)
	}
}

func TestFormatInt64(t *testing.T) {
	if formatInt64(3000000) != "3,000,000" {
		t.Fatal(formatInt64(3000000))
	}
}

func TestStackAboveRectNoObstacles(t *testing.T) {
	base := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	if got := stackAboveRect(base, nil, 4, 0); got != base {
		t.Fatalf("got %+v", got)
	}
}

func TestStackAboveRectCollapsedCompanion(t *testing.T) {
	// Both strips default to the same taskbar rectangle: the ZCode strip
	// must move directly above the companion strip with a gap.
	base := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	codex := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	got := stackAboveRect(base, []RECT{codex}, 4, 0)
	want := RECT{Left: 1680, Top: 956, Right: 1920, Bottom: 996}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
	// Touching edges are stable: a second pass must not move again.
	if again := stackAboveRect(got, []RECT{codex}, 4, 0); again != got {
		t.Fatalf("unstable: %+v -> %+v", got, again)
	}
}

func TestStackAboveRectIgnoresHorizontalMiss(t *testing.T) {
	base := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	far := RECT{Left: 0, Top: 1000, Right: 240, Bottom: 1040}
	if got := stackAboveRect(base, []RECT{far}, 4, 0); got != base {
		t.Fatalf("got %+v", got)
	}
}

func TestStackAboveRectChainsTwoCompanions(t *testing.T) {
	base := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	lower := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	upper := RECT{Left: 1680, Top: 956, Right: 1920, Bottom: 996}
	got := stackAboveRect(base, []RECT{lower, upper}, 4, 0)
	want := RECT{Left: 1680, Top: 912, Right: 1920, Bottom: 952}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestStackAboveRectClampsToMinY(t *testing.T) {
	base := RECT{Left: 1680, Top: 1000, Right: 1920, Bottom: 1040}
	tall := RECT{Left: 1600, Top: 0, Right: 1920, Bottom: 1030}
	got := stackAboveRect(base, []RECT{tall}, 4, 0)
	if got.Top < 0 || got.Bottom-got.Top != 40 {
		t.Fatalf("got %+v", got)
	}
}

func TestPeriodLabel(t *testing.T) {
	for in, want := range map[string]string{
		"daily": "DAILY", "weekly": "WEEKLY", "monthly": "MONTHLY",
		"annual": "YEARLY", "lifetime": "LIFETIME", "one_time": "ONE-TIME",
		"": "",
	} {
		if got := periodLabel(in); got != want {
			t.Fatalf("periodLabel(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPendingGrants(t *testing.T) {
	now := time.Now()
	// A 100M one-time promo plan whose bucket has not appeared yet.
	s := Snapshot{
		Plans: []Plan{{
			Name: "ZCode Global Build", UserPlanID: "upl-1", Status: "active",
			Entitlements: []Entitlement{{
				EntitlementID: "ent-promo", ShowName: "GLM-5.3-Flash",
				GrantUnits: 100000000, Period: "one_time", EffectiveAt: now.Add(2 * time.Hour),
				Capabilities: []string{"model:glm-5.3-flash"},
			}},
		}},
		Balances: []Balance{{ShowName: "GLM-5.3", EntitlementID: "ent-daily", Total: 3000000, Remaining: 3000000}},
	}
	pend := pendingGrants(s, now)
	if len(pend) != 1 {
		t.Fatalf("expected 1 pending grant, got %d", len(pend))
	}
	if pend[0].GrantUnits != 100000000 || pend[0].ShowName != "GLM-5.3-Flash" || pend[0].PlanName != "ZCode Global Build" {
		t.Fatalf("got %+v", pend[0])
	}
	// Once the bucket appears the grant is no longer pending.
	s.Balances = append(s.Balances, Balance{EntitlementID: "ent-promo", Total: 100000000, Remaining: 100000000})
	if pend := pendingGrants(s, now); len(pend) != 0 {
		t.Fatalf("expected 0 pending grants with live bucket, got %d", len(pend))
	}
	// Entitlements without an id are never tracked.
	s2 := Snapshot{Plans: []Plan{{Name: "x", Entitlements: []Entitlement{{GrantUnits: 5}}}}}
	if pend := pendingGrants(s2, now); len(pend) != 0 {
		t.Fatalf("id-less entitlement must be skipped, got %d", len(pend))
	}
}

func TestEntitlementGrantKey(t *testing.T) {
	if entitlementGrantKey("upl-1", "ent-1") != "upl-1|ent-1" {
		t.Fatal(entitlementGrantKey("upl-1", "ent-1"))
	}
}

func TestNewGrants(t *testing.T) {
	plans := []Plan{{
		UserPlanID: "upl-1", Name: "P",
		Entitlements: []Entitlement{{EntitlementID: "ent-a"}, {EntitlementID: "ent-b"}},
	}}
	known := map[string]bool{entitlementGrantKey("upl-1", "ent-a"): true}
	got := newGrants(plans, known)
	if len(got) != 1 || got[0].Ent.EntitlementID != "ent-b" {
		t.Fatalf("got %+v", got)
	}
	// An id-less entitlement is never reported as new.
	bad := []Plan{{UserPlanID: "u", Entitlements: []Entitlement{{}}}}
	if got := newGrants(bad, nil); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestPromoSeenStoreRoundTrip(t *testing.T) {
	path := t.TempDir() + "\\promotions-seen.json"
	m := map[string]bool{"a|b": true, "c|d": true}
	if err := savePromoSeenKeysAt(path, m); err != nil {
		t.Fatal(err)
	}
	got, valid := loadPromoSeenKeysAt(path)
	if !valid || len(got) != 2 || !got["a|b"] || !got["c|d"] {
		t.Fatalf("got %v valid=%v", got, valid)
	}
	// A missing file is an empty baseline, not an error.
	if got, valid := loadPromoSeenKeysAt(path + ".missing"); len(got) != 0 || valid {
		t.Fatalf("missing file: %v valid=%v", got, valid)
	}
	// A corrupt store must parse as absent so the HUD re-baselines
	// silently instead of re-toasting every grant.
	if err := os.WriteFile(path, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, valid := loadPromoSeenKeysAt(path); len(got) != 0 || valid {
		t.Fatalf("corrupt file: %v valid=%v", got, valid)
	}
}

func TestPromoToastBody(t *testing.T) {
	now := time.Now()
	r := planEntRef{
		Plan: Plan{Name: "ZCode Global Build"},
		Ent: Entitlement{
			ShowName: "GLM-5.3-Flash", GrantUnits: 100000000, Period: "one_time",
			EffectiveAt: now.Add(90 * time.Minute),
		},
	}
	body := promoToastBody(r, now)
	if !strings.Contains(body, "100,000,000") || !strings.Contains(body, "GLM-5.3-Flash") {
		t.Fatalf("body %q", body)
	}
	if !strings.Contains(body, "one-time") || !strings.Contains(body, "starts") {
		t.Fatalf("body %q", body)
	}
	// Already-effective grants say so instead of a past "starts".
	r.Ent.EffectiveAt = now.Add(-time.Hour)
	if body := promoToastBody(r, now); !strings.Contains(body, "already active") {
		t.Fatalf("body %q", body)
	}
	// Zero effective time omits the schedule clause.
	r.Ent.EffectiveAt = time.Time{}
	if body := promoToastBody(r, now); strings.Contains(body, "starts") || strings.Contains(body, "already active") {
		t.Fatalf("body %q", body)
	}
}

func TestPeriodQualifier(t *testing.T) {
	bs := []Balance{{Period: "daily"}, {Period: "Daily"}}
	if got := periodQualifier(bs); got != "(daily)" {
		t.Fatalf("got %q", got)
	}
	mixed := []Balance{{Period: "daily"}, {Period: "weekly"}}
	if got := periodQualifier(mixed); got != "(all)" {
		t.Fatalf("got %q", got)
	}
	if got := periodQualifier(nil); got != "(all)" {
		t.Fatalf("got %q", got)
	}
}

func TestFriendlyQuotaNote(t *testing.T) {
	if got := friendlyQuotaNote("当前用户不存在coding plan", false, ""); got != "Start Plan only — no coding plan" {
		t.Fatalf("got %q", got)
	}
	if got := friendlyQuotaNote("", true, "pro"); got != "Coding-plan level: pro" {
		t.Fatalf("got %q", got)
	}
	if got := friendlyQuotaNote("", true, ""); got != "Coding plan active" {
		t.Fatalf("got %q", got)
	}
}

func TestActivePlanPrefersActive(t *testing.T) {
	now := time.Now()
	s := Snapshot{Plans: []Plan{
		{Name: "old", Status: "expired", EndsAt: now.Add(-time.Hour)},
		{Name: "live", Status: "active", EndsAt: now.Add(time.Hour)},
	}}
	if got := s.activePlan(); got == nil || got.Name != "live" {
		t.Fatalf("got %+v", got)
	}
	if s2 := (Snapshot{}); s2.activePlan() != nil {
		t.Fatal("empty should be nil")
	}
}

func TestRemainingPctDrivesLeftSide(t *testing.T) {
	// % left must come from remaining_units, not 100-used.
	b := Balance{Total: 100, Used: 10, Remaining: 80}
	if got := remainingPct(b); got != 80 {
		t.Fatalf("got %v", got)
	}
}

func TestValidateAuthorizeURL(t *testing.T) {
	// The server-issued authorize URL must pass through untouched.
	in := "https://chat.z.ai/api/oauth/authorize?client_id=c&redirect_uri=https%3A%2F%2Fzcode.z.ai%2Fapi%2Fv1%2Foauth%2Fcli%2Fcallback%2Fzai&state=abc123&response_type=code"
	out, err := validateAuthorizeURL(in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("URL was rewritten: %s", out)
	}
	// The old desktop-bridge override is gone — the redirect stays the
	// server's own CLI callback.
	if strings.Contains(out, "app%2Foauth%2Flogin") || strings.Contains(out, "zcode%3A%2F%2F") {
		t.Fatalf("redirect was overridden: %s", out)
	}
	if _, err := validateAuthorizeURL("http://evil.example/x?state=s"); err == nil {
		t.Fatal("expected non-https error")
	}
	if _, err := validateAuthorizeURL("not a url"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestParseOAuthInit(t *testing.T) {
	now := time.Now().Unix()
	body := []byte(`{"code":0,"msg":"","data":{"flow_id":"f1","poll_token":"ptok","authorize_url":"https://chat.z.ai/api/oauth/authorize?client_id=c&redirect_uri=https%3A%2F%2Fzcode.z.ai%2Fapi%2Fv1%2Foauth%2Fcli%2Fcallback%2Fzai&state=s9&response_type=code","expires_at":` + strconv.FormatInt(now+300, 10) + `,"poll_interval_sec":2}}`)
	r, err := parseOAuthInit(body, "ptok")
	if err != nil {
		t.Fatal(err)
	}
	if r.flowID != "f1" || r.pollToken != "ptok" {
		t.Fatalf("got %+v", r)
	}
	if strings.Contains(r.authorizeURL, "app%2Foauth%2Flogin") {
		t.Fatalf("authorize URL was rewritten: %s", r.authorizeURL)
	}
	if r.pollURL != oauthPollURLPrefix+"f1" {
		t.Fatalf("pollURL %q", r.pollURL)
	}
	if r.pollInterval != 2*time.Second {
		t.Fatalf("interval %v", r.pollInterval)
	}
	bad := []byte(`{"code":0,"data":{"flow_id":"","authorize_url":"https://x/y","expires_at":1,"poll_interval_sec":0}}`)
	if _, err := parseOAuthInit(bad, "p"); err == nil {
		t.Fatal("expected bad-init error")
	}
}

func TestParseOAuthPoll(t *testing.T) {
	if st, ready, err := parseOAuthPoll([]byte(`{"code":0,"data":{"status":"pending"}}`)); err != nil || st != "pending" || ready != nil {
		t.Fatalf("got %q %v %v", st, ready, err)
	}
	if st, _, _ := parseOAuthPoll([]byte(`{"code":0,"data":{"status":"failed"}}`)); st != "failed" {
		t.Fatalf("got %q", st)
	}
	st, ready, err := parseOAuthPoll([]byte(`{"code":0,"data":{"status":"ready","token":"jwt1","zai":{"access_token":"oa1"},"user":{"user_id":"u","email":"e"}}}`))
	if err != nil || st != "ready" || ready == nil {
		t.Fatalf("got %q %v %v", st, ready, err)
	}
	if ready.zcodeJWT != "jwt1" || ready.oauthAccess != "oa1" || !strings.Contains(string(ready.userRaw), "u") {
		t.Fatalf("got %+v", ready)
	}
	if _, _, err := parseOAuthPoll([]byte(`{"code":0,"data":{"status":"ready","token":"","zai":{},"user":{}}}`)); err == nil {
		t.Fatal("expected incomplete-ready error")
	}
	if _, _, err := parseOAuthPoll([]byte(`{"code":5,"msg":"nope"}`)); err == nil {
		t.Fatal("expected code error")
	}
}

func TestParseBusinessToken(t *testing.T) {
	got, err := parseBusinessToken([]byte(`{"code":200,"success":true,"data":{"access_token":"biz1"}}`))
	if err != nil || got != "biz1" {
		t.Fatalf("got %q %v", got, err)
	}
	got, err = parseBusinessToken([]byte(`{"code":0,"data":{"accessToken":"biz2"}}`))
	if err != nil || got != "biz2" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := parseBusinessToken([]byte(`{"code":500,"success":false,"msg":"bad"}`)); err == nil {
		t.Fatal("expected failure")
	}
}

func TestCredentialEnvelopeRoundTrip(t *testing.T) {
	for _, plain := range []string{"zai", "eyJhbGciOiJIUzI1NiJ9.test", `{"user_id":"u"}`} {
		s, err := encryptCredential(plain)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(s, "enc:v1:") || len(strings.Split(s, ".")) != 3 {
			t.Fatalf("bad envelope %q", s)
		}
		back, err := decryptCredential(s)
		if err != nil || back != plain {
			t.Fatalf("round trip %q -> %q (%v)", plain, back, err)
		}
	}
}

func TestStripLoginKeys(t *testing.T) {
	m := map[string]string{
		"oauth:zai:access_token":  "a",
		"oauth:zai:refresh_token": "r",
		"zcodejwttoken":           "j",
		"oauth:zai:user_info":     "u",
		"oauth:active_provider":   "zai",
		"oauth:zai:other_thing":   "keep",
		"custom-key":              "keep",
	}
	got := stripLoginKeys(m)
	if len(got) != 2 || got["custom-key"] != "keep" || got["oauth:zai:other_thing"] != "keep" {
		t.Fatalf("got %v", got)
	}
	if len(m) != 7 {
		t.Fatal("input map must not be mutated")
	}
}

func TestClearLoginCredentialsAt(t *testing.T) {
	dir := t.TempDir()
	path := dir + "\\credentials.json"
	before := `{"oauth:zai:access_token":"a","zcodejwttoken":"j","oauth:zai:user_info":"u","oauth:active_provider":"zai","keep":"me"}`
	if err := os.WriteFile(path, []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if err := clearLoginCredentialsAt(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m["keep"] != "me" {
		t.Fatalf("got %v", m)
	}
	if err := clearLoginCredentialsAt(dir + "\\missing.json"); err != nil {
		t.Fatalf("missing file should be nil, got %v", err)
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret("abcdef1234567890"); got != "abcd...7890" {
		t.Fatalf("got %q", got)
	}
	if got := maskSecret("short"); got != "*****" {
		t.Fatalf("got %q", got)
	}
}

func TestMaskJSONForLogRedactsTokens(t *testing.T) {
	raw := []byte(`{"code":0,"data":{"access_token":"supersecret-token-value-12345","msg":"hello world, this message is intentionally made very long to exceed eighty characters total"}}`)
	got := maskJSONForLog(raw)
	if strings.Contains(got, "supersecret") || strings.Contains(got, "token-value") {
		t.Fatalf("token leaked: %s", got)
	}
	if !strings.Contains(got, "supe...2345") {
		t.Fatalf("expected masked token: %s", got)
	}
	if maskJSONForLog(nil) != "" {
		t.Fatal("empty should stay empty")
	}
}

func TestNewRequestIDShape(t *testing.T) {
	id := newRequestID()
	parts := strings.Split(id, "-")
	if len(parts) != 5 || len(id) != 36 {
		t.Fatalf("got %q", id)
	}
}

func TestHUDCredentialRoundTrip(t *testing.T) {
	dir := t.TempDir()
	creds := dir + "\\credentials.json"
	user := json.RawMessage(`{"user_id":"u1","email":"a@b.c","name":"N","avatar":"av"}`)
	if err := saveLoginCredentialsAt(creds, "jwt-abc", "biz-def", user); err != nil {
		t.Fatal(err)
	}
	got, err := loadCredsFromFiles(creds, dir+"\\no-telemetry.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.ZCodeJWT != "jwt-abc" || got.AccessToken != "biz-def" {
		t.Fatalf("got %+v", got)
	}
	if got.Email != "a@b.c" || got.Name != "N" || got.UserID != "u1" {
		t.Fatalf("got %+v", got)
	}
	if _, err := loadCredsFromFiles(dir+"\\missing.json", ""); err == nil {
		t.Fatal("expected not-exist error")
	}
}

func TestImportZCodeAppSessionFrom(t *testing.T) {
	dir := t.TempDir()
	src := dir + "\\src.json"
	dst := dir + "\\dst.json"
	srcMap := map[string]string{
		"oauth:zai:access_token": "tok",
		"zcodejwttoken":          "jwt",
		"oauth:zai:user_info":    `{"user_id":"u"}`,
		"oauth:active_provider":  "zai",
		"unrelated":              "x",
	}
	raw, _ := json.Marshal(srcMap)
	if err := os.WriteFile(src, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := importZCodeAppSessionFrom(src, dst); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	raw2, _ := os.ReadFile(dst)
	if err := json.Unmarshal(raw2, &m); err != nil {
		t.Fatal(err)
	}
	if m["oauth:zai:access_token"] != "tok" || m["zcodejwttoken"] != "jwt" {
		t.Fatalf("got %v", m)
	}
	if _, ok := m["unrelated"]; ok {
		t.Fatalf("unrelated key must not transfer: %v", m)
	}
	if err := importZCodeAppSessionFrom(dir+"\\absent.json", dst); err == nil {
		t.Fatal("expected error for missing source")
	}
	empty, _ := json.Marshal(map[string]string{"other": "1"})
	if err := os.WriteFile(src, empty, 0600); err != nil {
		t.Fatal(err)
	}
	if err := importZCodeAppSessionFrom(src, dst); err == nil {
		t.Fatal("expected error for session-less source")
	}
}

func TestUUIDv4(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id, err := uuidv4()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 36 || id[14] != '4' {
			t.Fatalf("not a v4 UUID: %q", id)
		}
		if m := map[byte]bool{'8': true, '9': true, 'a': true, 'b': true}; !m[id[19]] {
			t.Fatalf("bad variant nibble: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate UUID: %q", id)
		}
		seen[id] = true
	}
}

func TestResolveDeviceIDAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device-id.json")

	// App present → app id wins, nothing persisted.
	id, persisted, err := resolveDeviceIDAt(path, "app-mid")
	if err != nil || id != "app-mid" || persisted {
		t.Fatalf("app id: %q %v %v", id, persisted, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("app-mid case must not write the store: %v", err)
	}

	// App id whitespace-only → treated as absent.
	if id, _, _ := resolveDeviceIDAt(path, "   "); id == "" || id == "   " {
		t.Fatalf("whitespace app id must fall through, got %q", id)
	}

	// No app, no store → mint a UUID and persist it.
	id, _, err = resolveDeviceIDAt(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[14] != '4' {
		t.Fatalf("expected UUIDv4, got %q", id)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("minted id must persist: %v", err)
	}

	// Second call reuses the stored id.
	again, _, err := resolveDeviceIDAt(path, "")
	if err != nil || again != id {
		t.Fatalf("id not stable: %q vs %q (%v)", again, id, err)
	}

	// A stored HUD id loses to the app id once the app appears.
	if got, _, _ := resolveDeviceIDAt(path, "app-mid-2"); got != "app-mid-2" {
		t.Fatalf("app id must win over store: %q", got)
	}

	// Corrupt store → regenerate instead of failing.
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	fixed, _, err := resolveDeviceIDAt(path, "")
	if err != nil || len(fixed) != 36 {
		t.Fatalf("corrupt store must regenerate: %q %v", fixed, err)
	}

	// Unwritable store → still returns a usable per-launch id.
	ro := filepath.Join(dir, "ro-dir")
	if err := os.Mkdir(ro, 0500); err != nil {
		t.Fatal(err)
	}
	if id, _, _ := resolveDeviceIDAt(filepath.Join(ro, "device-id.json"), ""); id == "" {
		t.Fatal("unwritable store must still yield an id")
	}
}
