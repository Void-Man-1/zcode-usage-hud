# DESIGN.md — ZCode Usage HUD

Adopted from the **Linear.app** design reference (awesome-design-md library).
The HUD is a compact, always-on-top Windows widget, so the translation keeps
Linear's near-black canvas + hairline-panel language and its restrained use
of chromatic accents. Tokens below are the single source of truth for the
renderer's palette (`col*` vars in `main.go`).

## Colors

| Token | Hex | Use in the HUD |
| --- | --- | --- |
| `ink` | `#f7f8f8` | Title, section headers, countdown numbers, button labels |
| `ink-muted` | `#d0d6e0` | Values, bucket titles, stat values |
| `ink-subtle` | `#8a8f98` | Labels, meta text, footers, sync timestamps |
| `ink-tertiary` | `#62666d` | Deemptions, footnote rows, idle dot |
| `canvas` | `#010102` | Window background (never pure black) |
| `surface-1` | `#0f1011` | Sign-in panels, status panel, bucket cards |
| `surface-2` | `#141516` | Version chip, empty-state panels |
| `surface-4` | `#191a1b` | Minimize-button hover |
| `hairline` | `#23252a` | Card and panel borders |
| `hairline-strong` | `#34343a` | Version chip border, title-button hover edge |
| `hairline-tertiary` | `#3e3e44` | Sign-in button fill, progress-bar track |
| `primary` | `#5e6ad2` | Promo accent bars, PROMO qualifiers, plan note |
| `primary-hover` | `#828fff` | Lighter lavender for promo secondary text |
| `success` | `#27a644` | Available accent, healthy bars, online dot |
| `success-tint` | `#a9e8b5` | Readable success text on dark surfaces |
| `warn-text` | `#f2b65e` | Amber text: exhausted/expiring states |
| `warn-bar` | `#e8a74c` | Amber accent bars |
| `err-text` | `#f0947e` | Error and offline text |
| `err-bar` | `#cd4b4b` | Error bars, close-button hover |

## Rules

- The canvas is `#010102`, not pure black — the faint blue tint is intentional.
- Neutral inks and surfaces carry almost the entire UI; chromatic color is
  reserved for meaning: lavender marks promo/brand affordances, green marks
  available quota, amber marks exhausted or expiring pools, red marks errors.
- No light surfaces, gradients, or shadows; hierarchy comes from the surface
  ladder and hairline borders only.
- Semantic color must remain distinguishable by position and label, not hue
  alone (accessibility: percentages and status words always accompany color).
