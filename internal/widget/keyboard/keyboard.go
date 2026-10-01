package keyboard

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"phalune/internal/compositor"
	"phalune/internal/widget"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

var parensRegex = regexp.MustCompile(`\(([^)]+)\)`)

// languageCodeMap maps lowercase common language names or identifiers to their 2-letter uppercase codes.
var languageCodeMap = map[string]string{
	"english":    "US",
	"us":         "US",
	"usa":        "US",
	"persian":    "FA",
	"farsi":      "FA",
	"iran":       "FA",
	"ir":         "FA",
	"russian":    "RU",
	"ru":         "RU",
	"german":     "DE",
	"de":         "DE",
	"french":     "FR",
	"fr":         "FR",
	"spanish":    "ES",
	"es":         "ES",
	"italian":    "IT",
	"it":         "IT",
	"portuguese": "PT",
	"pt":         "PT",
	"arabic":     "AR",
	"ar":         "AR",
	"turkish":    "TR",
	"tr":         "TR",
	"chinese":    "ZH",
	"zh":         "ZH",
	"japanese":   "JA",
	"ja":         "JA",
	"korean":     "KO",
	"ko":         "KO",
	"ukrainian":  "UA",
	"ua":         "UA",
	"polish":     "PL",
	"pl":         "PL",
	"czech":      "CS",
	"cs":         "CS",
	"swedish":    "SV",
	"se":         "SV",
	"sv":         "SV",
	"finnish":    "FI",
	"fi":         "FI",
	"norwegian":  "NO",
	"no":         "NO",
	"danish":     "DA",
	"da":         "DA",
	"dutch":      "NL",
	"nl":         "NL",
	"greek":      "EL",
	"el":         "EL",
	"gr":         "EL",
	"hebrew":     "HE",
	"he":         "HE",
	"il":         "HE",
	"hindi":      "HI",
	"hi":         "HI",
	"hungarian":  "HU",
	"hu":         "HU",
	"romanian":   "RO",
	"ro":         "RO",
	"thai":       "TH",
	"th":         "TH",
	"vietnamese": "VI",
	"vi":         "VI",
}

// FormatLayoutName extracts a clean, uppercase short representation (e.g. "US", "FA", "RU") from a layout name.
func FormatLayoutName(fullName string) string {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return ""
	}

	// 1. Check parenthetical info like "English (US)" or "English (US, intl-altgr-dead-keys)"
	if m := parensRegex.FindStringSubmatch(fullName); len(m) > 1 {
		inside := strings.TrimSpace(m[1])
		parts := strings.Split(inside, ",")
		candidate := strings.TrimSpace(parts[0])
		if len(candidate) == 2 || len(candidate) == 3 {
			if code, ok := languageCodeMap[strings.ToLower(candidate)]; ok {
				return code
			}
			return strings.ToUpper(candidate[:2])
		}
		// If candidate is a layout name e.g. "AZERTY", check main language before parens
		beforeParens := strings.TrimSpace(strings.Split(fullName, "(")[0])
		if code, ok := languageCodeMap[strings.ToLower(beforeParens)]; ok {
			return code
		}
	}

	// 2. Direct lookup of whole name (e.g. "Persian", "Russian")
	lower := strings.ToLower(fullName)
	if code, ok := languageCodeMap[lower]; ok {
		return code
	}

	// 3. First word lookup (e.g. "Persian Standard" -> "Persian")
	fields := strings.Fields(lower)
	if len(fields) > 0 {
		if code, ok := languageCodeMap[fields[0]]; ok {
			return code
		}
	}

	// 4. Short strings (<= 3 characters) uppercase
	if len(fullName) <= 3 {
		return strings.ToUpper(fullName)
	}

	// 5. Fallback: first 2 characters uppercase
	return strings.ToUpper(fullName[:2])
}

type Keyboard struct {
	box           *gtk.Box
	icon          *gtk.Image
	label         *gtk.Label
	cancel        context.CancelFunc
	unsub         func()
	compositorSvc compositor.Service
	format        string
}

func New(ctx widget.Context) (widget.Widget, error) {
	builder := gtk.NewBuilderFromString(ui.Keyboard)
	box := builder.GetObject("keyboard_box").Cast().(*gtk.Box)
	var icon *gtk.Image
	if iconObj := builder.GetObject("keyboard_icon"); iconObj != nil {
		icon = iconObj.Cast().(*gtk.Image)
	}
	label := builder.GetObject("keyboard_label").Cast().(*gtk.Label)

	format := "%s"
	showIcon := false
	if ctx.Config != nil {
		if ctx.Config.Bar.Keyboard.Format != "" {
			format = ctx.Config.Bar.Keyboard.Format
		}
		showIcon = ctx.Config.Bar.Keyboard.ShowIcon
	}

	if icon != nil && !showIcon {
		icon.SetVisible(false)
	}

	kCtx, cancel := context.WithCancel(context.Background())
	k := &Keyboard{
		box:           box,
		icon:          icon,
		label:         label,
		cancel:        cancel,
		compositorSvc: ctx.Compositor,
		format:        format,
	}

	// Click switcher: switches to next layout
	click := gtk.NewGestureClick()
	click.SetButton(gdk.BUTTON_PRIMARY)
	click.ConnectReleased(func(n int, x, y float64) {
		if k.compositorSvc != nil {
			go func() {
				if err := k.compositorSvc.SwitchLayoutNext(); err != nil {
					slog.Warn("keyboard: failed to switch layout", "error", err)
				}
			}()
		}
	})
	box.AddController(click)

	// Subscribe to compositor keyboard layout events
	if ctx.Compositor != nil {
		ch, unsub := ctx.Compositor.SubscribeKeyboard()
		k.unsub = unsub
		go k.listenEvents(kCtx, ch)
	} else {
		label.SetLabel("US")
	}

	return k, nil
}

func (k *Keyboard) Root() gtk.Widgetter {
	return k.box
}

func (k *Keyboard) Destroy() {
	if k.cancel != nil {
		k.cancel()
		k.cancel = nil
	}
	if k.unsub != nil {
		k.unsub()
		k.unsub = nil
	}
}

func (k *Keyboard) listenEvents(ctx context.Context, ch <-chan compositor.KeyboardLayouts) {
	for {
		select {
		case <-ctx.Done():
			return
		case layouts, ok := <-ch:
			if !ok {
				return
			}
			k.update(layouts)
		}
	}
}

func (k *Keyboard) update(layouts compositor.KeyboardLayouts) {
	name := ""
	if layouts.CurrentIdx >= 0 && layouts.CurrentIdx < len(layouts.Names) {
		name = layouts.Names[layouts.CurrentIdx]
	}
	short := FormatLayoutName(name)
	if short == "" {
		short = "US"
	}

	text := short
	if k.format != "" {
		text = fmt.Sprintf(k.format, short)
	}

	glib.IdleAdd(func() {
		k.label.SetLabel(text)
		if name != "" {
			k.box.SetTooltipText(fmt.Sprintf("%s (%d/%d)", name, layouts.CurrentIdx+1, len(layouts.Names)))
		}
	})
}
