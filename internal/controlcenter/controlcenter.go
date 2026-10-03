package controlcenter

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"

	"phalune/internal/config"
	"phalune/internal/mpris"
	"phalune/internal/notify"
	"phalune/internal/style"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type ControlCenter struct {
	window       *gtk.Window
	overlayBox   *gtk.Overlay
	card         *gtk.Box
	contentStack *gtk.Stack

	// Quick toggles widgets
	wifiTile             *gtk.Box
	wifiButton           *gtk.Button
	wifiArrowButton      *gtk.Button
	wifiIcon             *gtk.Image
	wifiStatus           *gtk.Label
	bluetoothTile        *gtk.Box
	bluetoothButton      *gtk.Button
	bluetoothArrowButton *gtk.Button
	bluetoothIcon        *gtk.Image
	bluetoothStatus      *gtk.Label
	dndButton            *gtk.Button
	dndIcon              *gtk.Image
	dndStatus            *gtk.Label
	powerTile            *gtk.Box
	powerButton          *gtk.Button
	powerArrowButton     *gtk.Button
	powerIcon            *gtk.Image
	powerStatus          *gtk.Label

	// Volume slider controls
	volumeButton      *gtk.Button
	volumeArrowButton *gtk.Button

	// Wi-Fi subview widgets
	wifiBackButton     *gtk.Button
	wifiScanButton     *gtk.Button
	wifiRadioSwitch    *gtk.Switch
	wifiScrolledWindow *gtk.ScrolledWindow
	wifiListBox        *gtk.ListBox
	wifiEmptyLabel     *gtk.Label

	// Bluetooth subview widgets
	bluetoothBackButton     *gtk.Button
	bluetoothScanButton     *gtk.Button
	bluetoothRadioSwitch    *gtk.Switch
	bluetoothScrolledWindow *gtk.ScrolledWindow
	bluetoothListBox        *gtk.ListBox
	bluetoothEmptyLabel     *gtk.Label

	updatingWifiSwitch      bool
	updatingBluetoothSwitch bool

	// Power Profiles subview widgets
	powerBackButton *gtk.Button
	powerPerfBtn    *gtk.Button
	powerPerfCheck  *gtk.Image
	powerBalBtn     *gtk.Button
	powerBalCheck   *gtk.Image
	powerSaverBtn   *gtk.Button
	powerSaverCheck *gtk.Image

	// Audio subview widgets
	audioBackButton         *gtk.Button
	audioOutputButton       *gtk.Button
	audioOutputIcon         *gtk.Image
	audioOutputScale        *gtk.Scale
	audioOutputLabel        *gtk.Label
	audioInputButton        *gtk.Button
	audioInputIcon          *gtk.Image
	audioInputScale         *gtk.Scale
	audioInputLabel         *gtk.Label
	audioStreamsScrolledWin *gtk.ScrolledWindow
	audioStreamsListBox     *gtk.ListBox
	audioEmptyLabel         *gtk.Label
	inputBinding            *SliderBinding
	streamRows              map[int]*streamRow

	// Media Player (MPRIS) widgets
	mprisCard       *gtk.Box
	mprisArt        *gtk.Image
	mprisTitle      *gtk.Label
	mprisArtist     *gtk.Label
	mprisPlayerName *gtk.Label
	mprisPrevBtn    *gtk.Button
	mprisPlayBtn    *gtk.Button
	mprisNextBtn    *gtk.Button
	mprisDestroyed  bool
	mprisTexCache   map[string]*gdk.Texture
	mprisTexPath    string

	// Controllers
	wifiCtrl    *WiFiController
	btCtrl      *BluetoothController
	powerCtrl   *PowerController
	slidersCtrl *SlidersController
	audioCtrl   *AudioController
	mprisCtrl   *mpris.Controller
	notifyMgr   *notify.Manager

	mu             sync.Mutex
	visible        bool
	dndFallback    bool
	wifiPassSSID   string
	connectingSSID string
	wifiLastSig    string
}

type streamRow struct {
	binding      *SliderBinding
	cancelWorker context.CancelFunc
	listBoxRow   *gtk.ListBoxRow
	rowBox       *gtk.Box
	icon         *gtk.Image
	name         *gtk.Label
	media        *gtk.Label
	muteBtn      *gtk.Button
	muteIcon     *gtk.Image
}

func New(app *gtk.Application, notifyMgr *notify.Manager, cfg config.ControlCenterConfig) (*ControlCenter, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-control-center")
	win.SetDecorated(false)
	win.AddCSSClass("control-center-window")

	if err := ConfigureControlCenterSurface(win); err != nil {
		return nil, fmt.Errorf("failed to configure control-center layer-surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.ControlCenter)
	overlayBox := builder.GetObject("overlay_box").Cast().(*gtk.Overlay)
	card := builder.GetObject("card").Cast().(*gtk.Box)
	contentStack := builder.GetObject("content_stack").Cast().(*gtk.Stack)

	wifiTile := builder.GetObject("wifi_tile").Cast().(*gtk.Box)
	wifiBtn := builder.GetObject("wifi_button").Cast().(*gtk.Button)
	wifiArrowBtn := builder.GetObject("wifi_arrow_button").Cast().(*gtk.Button)
	wifiIcon := builder.GetObject("wifi_icon").Cast().(*gtk.Image)
	wifiStatus := builder.GetObject("wifi_status").Cast().(*gtk.Label)

	btTile := builder.GetObject("bluetooth_tile").Cast().(*gtk.Box)
	btBtn := builder.GetObject("bluetooth_button").Cast().(*gtk.Button)
	btArrowBtn := builder.GetObject("bluetooth_arrow_button").Cast().(*gtk.Button)
	btIcon := builder.GetObject("bluetooth_icon").Cast().(*gtk.Image)
	btStatus := builder.GetObject("bluetooth_status").Cast().(*gtk.Label)

	dndBtn := builder.GetObject("dnd_button").Cast().(*gtk.Button)
	dndIcon := builder.GetObject("dnd_icon").Cast().(*gtk.Image)
	dndStatus := builder.GetObject("dnd_status").Cast().(*gtk.Label)

	powerTile := builder.GetObject("power_tile").Cast().(*gtk.Box)
	powerBtn := builder.GetObject("power_button").Cast().(*gtk.Button)
	powerArrowBtn := builder.GetObject("power_arrow_button").Cast().(*gtk.Button)
	powerIcon := builder.GetObject("power_icon").Cast().(*gtk.Image)
	powerStatus := builder.GetObject("power_status").Cast().(*gtk.Label)

	volBtn := builder.GetObject("volume_button").Cast().(*gtk.Button)
	volIcon := builder.GetObject("volume_icon").Cast().(*gtk.Image)
	volScale := builder.GetObject("volume_scale").Cast().(*gtk.Scale)
	volLabel := builder.GetObject("volume_label").Cast().(*gtk.Label)
	volArrowBtn := builder.GetObject("volume_arrow_button").Cast().(*gtk.Button)

	briBtn := builder.GetObject("brightness_button").Cast().(*gtk.Button)
	briIcon := builder.GetObject("brightness_icon").Cast().(*gtk.Image)
	briScale := builder.GetObject("brightness_scale").Cast().(*gtk.Scale)
	briLabel := builder.GetObject("brightness_label").Cast().(*gtk.Label)

	wifiBackBtn := builder.GetObject("wifi_back_button").Cast().(*gtk.Button)
	wifiScanBtn := builder.GetObject("wifi_scan_button").Cast().(*gtk.Button)
	wifiRadioSwitch := builder.GetObject("wifi_radio_switch").Cast().(*gtk.Switch)
	wifiScrolledWin := builder.GetObject("wifi_scrolled_window").Cast().(*gtk.ScrolledWindow)
	wifiListBox := builder.GetObject("wifi_list_box").Cast().(*gtk.ListBox)
	wifiEmptyLabel := builder.GetObject("wifi_empty_label").Cast().(*gtk.Label)

	btBackBtn := builder.GetObject("bluetooth_back_button").Cast().(*gtk.Button)
	btScanBtn := builder.GetObject("bluetooth_scan_button").Cast().(*gtk.Button)
	btRadioSwitch := builder.GetObject("bluetooth_radio_switch").Cast().(*gtk.Switch)
	btScrolledWin := builder.GetObject("bluetooth_scrolled_window").Cast().(*gtk.ScrolledWindow)
	btListBox := builder.GetObject("bluetooth_list_box").Cast().(*gtk.ListBox)
	btEmptyLabel := builder.GetObject("bluetooth_empty_label").Cast().(*gtk.Label)

	powerBackBtn := builder.GetObject("power_back_button").Cast().(*gtk.Button)
	powerPerfBtn := builder.GetObject("power_profile_performance_btn").Cast().(*gtk.Button)
	powerPerfCheck := builder.GetObject("power_profile_performance_check").Cast().(*gtk.Image)
	powerBalBtn := builder.GetObject("power_profile_balanced_btn").Cast().(*gtk.Button)
	powerBalCheck := builder.GetObject("power_profile_balanced_check").Cast().(*gtk.Image)
	powerSaverBtn := builder.GetObject("power_profile_saver_btn").Cast().(*gtk.Button)
	powerSaverCheck := builder.GetObject("power_profile_saver_check").Cast().(*gtk.Image)

	audioBackBtn := builder.GetObject("audio_back_button").Cast().(*gtk.Button)
	audioOutBtn := builder.GetObject("audio_output_button").Cast().(*gtk.Button)
	audioOutIcon := builder.GetObject("audio_output_icon").Cast().(*gtk.Image)
	audioOutScale := builder.GetObject("audio_output_scale").Cast().(*gtk.Scale)
	audioOutLabel := builder.GetObject("audio_output_label").Cast().(*gtk.Label)
	audioInBtn := builder.GetObject("audio_input_button").Cast().(*gtk.Button)
	audioInIcon := builder.GetObject("audio_input_icon").Cast().(*gtk.Image)
	audioInScale := builder.GetObject("audio_input_scale").Cast().(*gtk.Scale)
	audioInLabel := builder.GetObject("audio_input_label").Cast().(*gtk.Label)
	audioScrolledWin := builder.GetObject("audio_streams_scrolled_window").Cast().(*gtk.ScrolledWindow)
	audioListBox := builder.GetObject("audio_streams_list_box").Cast().(*gtk.ListBox)
	audioEmptyLabel := builder.GetObject("audio_empty_label").Cast().(*gtk.Label)

	mprisCard := builder.GetObject("mpris_card").Cast().(*gtk.Box)
	mprisArt := builder.GetObject("mpris_art").Cast().(*gtk.Image)
	mprisTitle := builder.GetObject("mpris_title").Cast().(*gtk.Label)
	mprisArtist := builder.GetObject("mpris_artist").Cast().(*gtk.Label)
	mprisPlayerName := builder.GetObject("mpris_player_name").Cast().(*gtk.Label)
	mprisPrevBtn := builder.GetObject("mpris_prev_button").Cast().(*gtk.Button)
	mprisPlayBtn := builder.GetObject("mpris_play_button").Cast().(*gtk.Button)
	mprisNextBtn := builder.GetObject("mpris_next_button").Cast().(*gtk.Button)

	win.SetChild(overlayBox)

	cc := &ControlCenter{
		window:                  win,
		overlayBox:              overlayBox,
		card:                    card,
		contentStack:            contentStack,
		wifiTile:                wifiTile,
		wifiButton:              wifiBtn,
		wifiArrowButton:         wifiArrowBtn,
		wifiIcon:                wifiIcon,
		wifiStatus:              wifiStatus,
		bluetoothTile:           btTile,
		bluetoothButton:         btBtn,
		bluetoothArrowButton:    btArrowBtn,
		bluetoothIcon:           btIcon,
		bluetoothStatus:         btStatus,
		dndButton:               dndBtn,
		dndIcon:                 dndIcon,
		dndStatus:               dndStatus,
		powerTile:               powerTile,
		powerButton:             powerBtn,
		powerArrowButton:        powerArrowBtn,
		powerIcon:               powerIcon,
		powerStatus:             powerStatus,
		volumeButton:            volBtn,
		volumeArrowButton:       volArrowBtn,
		powerBackButton:         powerBackBtn,
		powerPerfBtn:            powerPerfBtn,
		powerPerfCheck:          powerPerfCheck,
		powerBalBtn:             powerBalBtn,
		powerBalCheck:           powerBalCheck,
		powerSaverBtn:           powerSaverBtn,
		powerSaverCheck:         powerSaverCheck,
		audioBackButton:         audioBackBtn,
		audioOutputButton:       audioOutBtn,
		audioOutputIcon:         audioOutIcon,
		audioOutputScale:        audioOutScale,
		audioOutputLabel:        audioOutLabel,
		audioInputButton:        audioInBtn,
		audioInputIcon:          audioInIcon,
		audioInputScale:         audioInScale,
		audioInputLabel:         audioInLabel,
		audioStreamsScrolledWin: audioScrolledWin,
		audioStreamsListBox:     audioListBox,
		audioEmptyLabel:         audioEmptyLabel,
		wifiBackButton:          wifiBackBtn,
		wifiScanButton:          wifiScanBtn,
		wifiRadioSwitch:         wifiRadioSwitch,
		wifiScrolledWindow:      wifiScrolledWin,
		wifiListBox:             wifiListBox,
		wifiEmptyLabel:          wifiEmptyLabel,
		bluetoothBackButton:     btBackBtn,
		bluetoothScanButton:     btScanBtn,
		bluetoothRadioSwitch:    btRadioSwitch,
		bluetoothScrolledWindow: btScrolledWin,
		bluetoothListBox:        btListBox,
		bluetoothEmptyLabel:     btEmptyLabel,
		mprisCard:               mprisCard,
		mprisArt:                mprisArt,
		mprisTitle:              mprisTitle,
		mprisArtist:             mprisArtist,
		mprisPlayerName:         mprisPlayerName,
		mprisPrevBtn:            mprisPrevBtn,
		mprisPlayBtn:            mprisPlayBtn,
		mprisNextBtn:            mprisNextBtn,
		mprisTexCache:           make(map[string]*gdk.Texture),
		notifyMgr:               notifyMgr,
	}

	styleName, styleClass := style.Resolve(style.ControlCenter, cfg.Style)
	cc.applyStyle(styleName, styleClass)

	cc.wifiCtrl = NewWiFiController(func(enabled bool, available bool, subtitle, icon string) {
		cc.wifiStatus.SetText(subtitle)
		cc.wifiIcon.SetFromIconName(icon)
		if cc.wifiRadioSwitch.Active() != enabled {
			cc.updatingWifiSwitch = true
			cc.wifiRadioSwitch.SetActive(enabled)
			cc.updatingWifiSwitch = false
		}
		if enabled {
			cc.wifiTile.AddCSSClass("active")
			cc.wifiButton.AddCSSClass("active")
		} else {
			cc.wifiTile.RemoveCSSClass("active")
			cc.wifiButton.RemoveCSSClass("active")
		}
	})

	cc.wifiCtrl.SetOnNetworksChanged(func(networks []AccessPoint) {
		cc.renderNetworks(networks)
	})

	cc.btCtrl = NewBluetoothController(func(powered, available bool, title, icon string) {
		cc.bluetoothStatus.SetText(title)
		cc.bluetoothIcon.SetFromIconName(icon)
		if cc.bluetoothRadioSwitch.Active() != powered {
			cc.updatingBluetoothSwitch = true
			cc.bluetoothRadioSwitch.SetActive(powered)
			cc.updatingBluetoothSwitch = false
		}
		if powered {
			cc.bluetoothTile.AddCSSClass("active")
			cc.bluetoothButton.AddCSSClass("active")
		} else {
			cc.bluetoothTile.RemoveCSSClass("active")
			cc.bluetoothButton.RemoveCSSClass("active")
		}
	})

	cc.btCtrl.SetOnDevicesChanged(func(devices []BluetoothDevice) {
		cc.renderBluetoothDevices(devices)
	})

	cc.powerCtrl = NewPowerController(func(profile, title, icon string, active bool) {
		cc.powerStatus.SetText(title)
		cc.powerIcon.SetFromIconName(icon)
		if active {
			cc.powerTile.AddCSSClass("active")
			cc.powerButton.AddCSSClass("active")
		} else {
			cc.powerTile.RemoveCSSClass("active")
			cc.powerButton.RemoveCSSClass("active")
		}
		cc.updatePowerProfilesUI(profile)
	})

	cc.slidersCtrl = NewSlidersController(
		volBtn,
		volIcon,
		volScale,
		volLabel,
		briBtn,
		briIcon,
		briScale,
		briLabel,
	)
	cc.slidersCtrl.BindSubpageOutput(audioOutBtn, audioOutIcon, audioOutScale, audioOutLabel)

	cc.streamRows = make(map[int]*streamRow)
	cc.inputBinding = NewSliderBinding(SliderConfig{
		Scale:  audioInScale,
		Label:  audioInLabel,
		Button: audioInBtn,
		Icon:   audioInIcon,
		OnApply: func(pct int) {
			cc.audioCtrl.SetInputVolume(pct)
		},
		OnMute: func() {
			cc.audioCtrl.ToggleInputMute()
		},
		GetIcon: func(pct int, muted bool) string {
			return InputVolumeIconName(pct, muted)
		},
	})

	cc.audioCtrl = NewAudioController()
	cc.audioCtrl.SetOnInputChanged(func(vol float64, muted bool) {
		cc.updateInputUI(vol, muted)
	})
	cc.audioCtrl.SetOnStreamsChanged(func(streams []AudioStream) {
		cc.renderAudioStreams(streams)
	})

	if mprisCtrl, err := mpris.New(); err == nil {
		cc.mprisCtrl = mprisCtrl
		cc.mprisCtrl.OnChange(func(active *mpris.PlayerState) {
			glib.IdleAdd(func() {
				cc.updateMprisUI(active)
			})
		})
	} else {
		slog.Warn("controlcenter: mpris unavailable", "error", err)
	}

	cc.setupInteractivity()

	return cc, nil
}

func attachSecondaryClick(btn *gtk.Button, onSecondary func()) {
	if btn == nil {
		return
	}
	gesture := gtk.NewGestureClick()
	gesture.SetButton(gdk.BUTTON_SECONDARY)
	gesture.ConnectReleased(func(n int, x, y float64) {
		onSecondary()
	})
	btn.AddController(gesture)
}

func (cc *ControlCenter) setupInteractivity() {
	// Wi-Fi tile main button (toggle radio on left click, open subpage on right click)
	cc.wifiButton.ConnectClicked(func() {
		cc.wifiCtrl.Toggle()
	})
	attachSecondaryClick(cc.wifiButton, func() {
		cc.OpenSubpage("wifi")
	})

	// Wi-Fi arrow button (open subview)
	cc.wifiArrowButton.ConnectClicked(func() {
		cc.OpenSubpage("wifi")
	})

	// Wi-Fi subview header controls
	cc.wifiBackButton.ConnectClicked(func() {
		cc.contentStack.SetVisibleChildName("main")
	})

	cc.wifiScanButton.ConnectClicked(func() {
		cc.wifiCtrl.Scan()
	})

	cc.wifiRadioSwitch.ConnectStateSet(func(state bool) bool {
		if cc.updatingWifiSwitch {
			return false
		}
		cc.wifiCtrl.SetWirelessEnabled(state)
		return false
	})

	// Bluetooth tile main button (toggle adapter on left click, open subpage on right click)
	cc.bluetoothButton.ConnectClicked(func() {
		cc.btCtrl.Toggle()
	})
	attachSecondaryClick(cc.bluetoothButton, func() {
		cc.OpenSubpage("bluetooth")
	})

	// Bluetooth arrow button (open subview)
	cc.bluetoothArrowButton.ConnectClicked(func() {
		cc.OpenSubpage("bluetooth")
	})

	// Bluetooth subview header controls
	cc.bluetoothBackButton.ConnectClicked(func() {
		cc.contentStack.SetVisibleChildName("main")
	})

	cc.bluetoothScanButton.ConnectClicked(func() {
		cc.btCtrl.Scan()
	})

	cc.bluetoothRadioSwitch.ConnectStateSet(func(state bool) bool {
		if cc.updatingBluetoothSwitch {
			return false
		}
		cc.btCtrl.SetPowered(state)
		return false
	})

	// DND click
	cc.dndButton.ConnectClicked(func() {
		cc.toggleDND()
	})

	// Power Mode click (cycle profile on left click, open subpage on right click)
	cc.powerButton.ConnectClicked(func() {
		cc.powerCtrl.Toggle()
	})
	attachSecondaryClick(cc.powerButton, func() {
		cc.OpenSubpage("power")
	})

	// Power arrow button (open subview)
	cc.powerArrowButton.ConnectClicked(func() {
		cc.OpenSubpage("power")
	})

	// Power subview controls
	cc.powerBackButton.ConnectClicked(func() {
		cc.contentStack.SetVisibleChildName("main")
	})

	cc.powerPerfBtn.ConnectClicked(func() {
		cc.powerCtrl.SetProfile(ProfilePerformance)
	})
	cc.powerBalBtn.ConnectClicked(func() {
		cc.powerCtrl.SetProfile(ProfileBalanced)
	})
	cc.powerSaverBtn.ConnectClicked(func() {
		cc.powerCtrl.SetProfile(ProfilePowerSaver)
	})

	// Volume button (toggle mute on left click, open audio subview on right click)
	if cc.volumeButton != nil {
		attachSecondaryClick(cc.volumeButton, func() {
			cc.OpenSubpage("audio")
		})
	}

	// Volume arrow button (open subview)
	cc.volumeArrowButton.ConnectClicked(func() {
		cc.OpenSubpage("audio")
	})

	// Audio subview controls
	cc.audioBackButton.ConnectClicked(func() {
		cc.contentStack.SetVisibleChildName("main")
	})

	// MPRIS controls
	cc.mprisPrevBtn.ConnectClicked(func() {
		if cc.mprisCtrl != nil {
			_ = cc.mprisCtrl.Previous()
		}
	})
	cc.mprisPlayBtn.ConnectClicked(func() {
		if cc.mprisCtrl != nil {
			_ = cc.mprisCtrl.PlayPause()
		}
	})
	cc.mprisNextBtn.ConnectClicked(func() {
		if cc.mprisCtrl != nil {
			_ = cc.mprisCtrl.Next()
		}
	})

	// Outside click dismissal
	click := gtk.NewGestureClick()
	click.ConnectReleased(func(n int, x, y float64) {
		pick := cc.overlayBox.Pick(x, y, gtk.PickDefault)
		if pick != nil {
			w := gtk.BaseWidget(pick)
			if w != nil && (w == &cc.card.Widget || w.IsAncestor(cc.card)) {
				return
			}
		}
		cc.Close()
	})
	cc.overlayBox.AddController(click)

	// Escape key navigation
	keyCtrl := gtk.NewEventControllerKey()
	keyCtrl.ConnectKeyPressed(func(keyval, keycode uint, state gdk.ModifierType) bool {
		if keyval == gdk.KEY_Escape {
			if cc.contentStack != nil && cc.contentStack.VisibleChildName() != "main" {
				cc.contentStack.SetVisibleChildName("main")
				return true
			}
			cc.Close()
			return true
		}
		return false
	})
	cc.window.AddController(keyCtrl)
}

func wifiNetworksSignature(networks []AccessPoint, connectingSSID, passSSID string) string {
	var sb strings.Builder
	sb.WriteString(connectingSSID)
	sb.WriteByte('|')
	sb.WriteString(passSSID)
	sb.WriteByte('|')
	for _, ap := range networks {
		sb.WriteString(ap.SSID)
		if ap.Connected {
			sb.WriteString(":C")
		}
		if ap.Saved {
			sb.WriteString(":S")
		}
		sb.WriteString(WifiSignalIcon(ap.Strength))
		sb.WriteByte(';')
	}
	return sb.String()
}

func (cc *ControlCenter) renderNetworks(networks []AccessPoint) {
	if len(networks) == 0 {
		for row := cc.wifiListBox.RowAtIndex(0); row != nil; row = cc.wifiListBox.RowAtIndex(0) {
			cc.wifiListBox.Remove(row)
		}
		cc.wifiEmptyLabel.SetVisible(true)
		cc.wifiScrolledWindow.SetVisible(false)
		cc.wifiLastSig = ""
		return
	}

	sig := wifiNetworksSignature(networks, cc.connectingSSID, cc.wifiPassSSID)
	if sig == cc.wifiLastSig {
		return
	}
	cc.wifiLastSig = sig

	for row := cc.wifiListBox.RowAtIndex(0); row != nil; row = cc.wifiListBox.RowAtIndex(0) {
		cc.wifiListBox.Remove(row)
	}

	cc.wifiEmptyLabel.SetVisible(false)
	cc.wifiScrolledWindow.SetVisible(true)

	var savedNets []AccessPoint
	var unsavedNets []AccessPoint

	for _, net := range networks {
		if net.Connected || net.Saved {
			savedNets = append(savedNets, net)
		} else {
			unsavedNets = append(unsavedNets, net)
		}
	}

	renderItem := func(ap AccessPoint, isSavedSection bool) {
		builder := gtk.NewBuilderFromString(ui.WifiItem)
		rowBox := builder.GetObject("wifi_item_box").Cast().(*gtk.Box)
		signalImg := builder.GetObject("wifi_item_signal").Cast().(*gtk.Image)
		ssidLabel := builder.GetObject("wifi_item_ssid").Cast().(*gtk.Label)
		statusLabel := builder.GetObject("wifi_item_status").Cast().(*gtk.Label)
		lockImg := builder.GetObject("wifi_item_lock").Cast().(*gtk.Image)
		connImg := builder.GetObject("wifi_item_connected").Cast().(*gtk.Image)
		actionBtn := builder.GetObject("wifi_item_button").Cast().(*gtk.Button)
		pwdBox := builder.GetObject("wifi_password_box").Cast().(*gtk.Box)
		pwdEntry := builder.GetObject("wifi_password_entry").Cast().(*gtk.PasswordEntry)
		joinBtn := builder.GetObject("wifi_password_connect_btn").Cast().(*gtk.Button)
		cancelBtn := builder.GetObject("wifi_password_cancel_btn").Cast().(*gtk.Button)

		ssidLabel.SetText(ap.SSID)
		signalImg.SetFromIconName(WifiSignalIcon(ap.Strength))
		lockImg.SetVisible(ap.Secured)
		pwdBox.SetVisible(false)

		doConnect := func(pass string) {
			cc.connectingSSID = ap.SSID
			cc.wifiLastSig = ""
			actionBtn.SetSensitive(false)
			statusLabel.SetText("Connecting...")
			statusLabel.SetVisible(true)
			go func() {
				err := cc.wifiCtrl.Connect(ap.SSID, pass)
				glib.IdleAdd(func() {
					if err != nil {
						if cc.connectingSSID == ap.SSID {
							cc.connectingSSID = ""
							cc.wifiLastSig = ""
						}
						actionBtn.SetSensitive(true)
						statusLabel.SetText("Failed to connect")
					}
				})
			}()
		}

		if ap.Connected {
			if cc.connectingSSID == ap.SSID {
				cc.connectingSSID = ""
			}
			rowBox.AddCSSClass("connected")
			connImg.SetVisible(true)
			statusLabel.SetText("Connected")
			statusLabel.SetVisible(true)
			actionBtn.SetLabel("Disconnect")
			actionBtn.AddCSSClass("destructive-action")
			actionBtn.ConnectClicked(func() {
				actionBtn.SetSensitive(false)
				go func() {
					_ = cc.wifiCtrl.Disconnect()
					glib.IdleAdd(func() {
						actionBtn.SetSensitive(true)
					})
				}()
			})
		} else if isSavedSection {
			rowBox.RemoveCSSClass("connected")
			connImg.SetVisible(false)
			actionBtn.RemoveCSSClass("destructive-action")
			if cc.connectingSSID == ap.SSID {
				actionBtn.SetLabel("Connecting...")
				actionBtn.SetSensitive(false)
				statusLabel.SetText("Connecting...")
				statusLabel.SetVisible(true)
			} else {
				actionBtn.SetLabel("Connect")
				actionBtn.SetSensitive(true)
				statusLabel.SetVisible(false)
			}
			actionBtn.ConnectClicked(func() {
				doConnect("")
			})
		} else {
			rowBox.RemoveCSSClass("connected")
			connImg.SetVisible(false)
			actionBtn.RemoveCSSClass("destructive-action")
			if cc.connectingSSID == ap.SSID {
				actionBtn.SetLabel("Connecting...")
				actionBtn.SetSensitive(false)
				statusLabel.SetText("Connecting...")
				statusLabel.SetVisible(true)
			} else {
				actionBtn.SetLabel("Connect")
				actionBtn.SetSensitive(true)
				statusLabel.SetVisible(false)
			}

			if !ap.Secured {
				actionBtn.ConnectClicked(func() {
					doConnect("")
				})
			} else {
				if cc.wifiPassSSID == ap.SSID {
					pwdBox.SetVisible(true)
					pwdEntry.GrabFocus()
				}

				actionBtn.ConnectClicked(func() {
					if cc.wifiPassSSID == ap.SSID {
						cc.wifiPassSSID = ""
						pwdBox.SetVisible(false)
						cc.wifiLastSig = ""
					} else {
						cc.wifiPassSSID = ap.SSID
						pwdBox.SetVisible(true)
						pwdEntry.GrabFocus()
						cc.wifiLastSig = ""
					}
				})

				joinBtn.ConnectClicked(func() {
					cc.wifiPassSSID = ""
					pwdBox.SetVisible(false)
					cc.wifiLastSig = ""
					doConnect(pwdEntry.Text())
				})

				pwdEntry.ConnectActivate(func() {
					cc.wifiPassSSID = ""
					pwdBox.SetVisible(false)
					cc.wifiLastSig = ""
					doConnect(pwdEntry.Text())
				})

				cancelBtn.ConnectClicked(func() {
					cc.wifiPassSSID = ""
					pwdBox.SetVisible(false)
					cc.wifiLastSig = ""
				})
			}
		}

		cc.wifiListBox.Append(rowBox)
	}

	for _, ap := range savedNets {
		renderItem(ap, true)
	}
	for _, ap := range unsavedNets {
		renderItem(ap, false)
	}
}

func (cc *ControlCenter) renderBluetoothDevices(devices []BluetoothDevice) {
	for row := cc.bluetoothListBox.RowAtIndex(0); row != nil; row = cc.bluetoothListBox.RowAtIndex(0) {
		cc.bluetoothListBox.Remove(row)
	}

	if len(devices) == 0 {
		cc.bluetoothEmptyLabel.SetVisible(true)
		cc.bluetoothScrolledWindow.SetVisible(false)
		return
	}

	cc.bluetoothEmptyLabel.SetVisible(false)
	cc.bluetoothScrolledWindow.SetVisible(true)

	for _, dev := range devices {
		d := dev
		builder := gtk.NewBuilderFromString(ui.BluetoothItem)
		rowBox := builder.GetObject("bluetooth_item_box").Cast().(*gtk.Box)
		devIcon := builder.GetObject("bluetooth_item_icon").Cast().(*gtk.Image)
		nameLabel := builder.GetObject("bluetooth_item_name").Cast().(*gtk.Label)
		statusLabel := builder.GetObject("bluetooth_item_status").Cast().(*gtk.Label)
		connImg := builder.GetObject("bluetooth_item_connected").Cast().(*gtk.Image)
		actionBtn := builder.GetObject("bluetooth_item_button").Cast().(*gtk.Button)

		nameLabel.SetText(d.Name)
		devIcon.SetFromIconName(BluetoothDeviceIcon(d.Icon))

		if d.Connected {
			rowBox.AddCSSClass("connected")
			connImg.SetVisible(true)
			statusLabel.SetText("Connected")
			statusLabel.SetVisible(true)
			actionBtn.SetLabel("Disconnect")
			actionBtn.AddCSSClass("destructive-action")
			actionBtn.ConnectClicked(func() {
				actionBtn.SetSensitive(false)
				go func() {
					_ = cc.btCtrl.Disconnect(d.Address)
					glib.IdleAdd(func() {
						actionBtn.SetSensitive(true)
					})
				}()
			})
		} else {
			rowBox.RemoveCSSClass("connected")
			connImg.SetVisible(false)
			if d.Paired {
				statusLabel.SetText("Paired")
				statusLabel.SetVisible(true)
			} else {
				statusLabel.SetVisible(false)
			}
			actionBtn.SetLabel("Connect")
			actionBtn.RemoveCSSClass("destructive-action")
			actionBtn.ConnectClicked(func() {
				actionBtn.SetSensitive(false)
				statusLabel.SetText("Connecting...")
				statusLabel.SetVisible(true)
				go func() {
					err := cc.btCtrl.Connect(d.Address)
					glib.IdleAdd(func() {
						actionBtn.SetSensitive(true)
						if err != nil {
							statusLabel.SetText("Connection failed")
						}
					})
				}()
			})
		}

		actionsBox := builder.GetObject("bluetooth_actions_box").Cast().(*gtk.Box)
		forgetBtn := builder.GetObject("bluetooth_forget_btn").Cast().(*gtk.Button)
		cancelBtn := builder.GetObject("bluetooth_cancel_btn").Cast().(*gtk.Button)

		if d.Paired {
			forgetBtn.ConnectClicked(func() {
				forgetBtn.SetSensitive(false)
				go func() {
					_ = cc.btCtrl.Forget(d.Address)
				}()
			})
			cancelBtn.ConnectClicked(func() {
				actionsBox.SetVisible(false)
			})
		}

		cc.bluetoothListBox.Append(rowBox)
	}
}

func (cc *ControlCenter) toggleDND() {
	var next bool
	if cc.notifyMgr != nil {
		next = !cc.notifyMgr.IsDND()
		cc.notifyMgr.SetDND(next)
	} else {
		cc.mu.Lock()
		cc.dndFallback = !cc.dndFallback
		next = cc.dndFallback
		cc.mu.Unlock()
	}
	cc.updateDNDUI(next)
}

func (cc *ControlCenter) updateDNDUI(enabled bool) {
	if enabled {
		cc.dndButton.AddCSSClass("active")
		cc.dndStatus.SetText("On")
		cc.dndIcon.SetFromIconName("notifications-disabled-symbolic")
	} else {
		cc.dndButton.RemoveCSSClass("active")
		cc.dndStatus.SetText("Off")
		cc.dndIcon.SetFromIconName("preferences-system-notifications-symbolic")
	}
}

func (cc *ControlCenter) SetShowOSD(fn func(icon, label string, value float64)) {
	if cc.slidersCtrl != nil {
		cc.slidersCtrl.SetShowOSD(fn)
	}
}

func (cc *ControlCenter) Start(ctx context.Context) {
	if cc.notifyMgr != nil {
		cc.updateDNDUI(cc.notifyMgr.IsDND())
	} else {
		cc.mu.Lock()
		cc.updateDNDUI(cc.dndFallback)
		cc.mu.Unlock()
	}

	cc.wifiCtrl.Start(ctx)
	cc.btCtrl.Start(ctx)
	cc.powerCtrl.Start(ctx)
	cc.slidersCtrl.Start(ctx)
	cc.audioCtrl.Start(ctx)
	cc.inputBinding.StartWorker(ctx)
}

func (cc *ControlCenter) Open() {
	glib.IdleAdd(func() {
		cc.mu.Lock()
		cc.visible = true
		cc.mu.Unlock()

		if cc.notifyMgr != nil {
			cc.updateDNDUI(cc.notifyMgr.IsDND())
		} else {
			cc.mu.Lock()
			cc.updateDNDUI(cc.dndFallback)
			cc.mu.Unlock()
		}

		if cc.slidersCtrl != nil {
			cc.slidersCtrl.Sync()
		}

		if cc.contentStack != nil {
			cc.contentStack.SetVisibleChildName("main")
		}

		if cc.wifiCtrl != nil {
			go cc.wifiCtrl.Refresh()
		}

		cc.window.SetVisible(true)
		cc.window.Present()
	})
}

func (cc *ControlCenter) OpenSubpage(page string) {
	glib.IdleAdd(func() {
		cc.mu.Lock()
		cc.visible = true
		cc.mu.Unlock()

		if cc.notifyMgr != nil {
			cc.updateDNDUI(cc.notifyMgr.IsDND())
		} else {
			cc.mu.Lock()
			cc.updateDNDUI(cc.dndFallback)
			cc.mu.Unlock()
		}

		if cc.slidersCtrl != nil {
			cc.slidersCtrl.Sync()
		}

		if cc.contentStack != nil {
			cc.contentStack.SetVisibleChildName(page)
		}

		if page == "wifi" && cc.wifiCtrl != nil {
			cc.wifiLastSig = ""
			cc.wifiCtrl.Scan()
		} else if page == "bluetooth" && cc.btCtrl != nil {
			cc.btCtrl.Scan()
		} else if page == "audio" && cc.audioCtrl != nil {
			cc.audioCtrl.Refresh()
		} else if page == "power" && cc.powerCtrl != nil {
			cc.updatePowerProfilesUI(cc.powerCtrl.CurrentProfile())
		}

		cc.window.SetVisible(true)
		cc.window.Present()
	})
}

func (cc *ControlCenter) updatePowerProfilesUI(profile string) {
	norm := strings.ToLower(strings.TrimSpace(profile))
	isPerf := norm == ProfilePerformance
	isBal := norm == ProfileBalanced || norm == ""
	isSaver := norm == ProfilePowerSaver

	if cc.powerPerfCheck != nil {
		cc.powerPerfCheck.SetVisible(isPerf)
	}
	if cc.powerBalCheck != nil {
		cc.powerBalCheck.SetVisible(isBal)
	}
	if cc.powerSaverCheck != nil {
		cc.powerSaverCheck.SetVisible(isSaver)
	}

	if cc.powerPerfBtn != nil {
		if isPerf {
			cc.powerPerfBtn.AddCSSClass("active")
		} else {
			cc.powerPerfBtn.RemoveCSSClass("active")
		}
	}
	if cc.powerBalBtn != nil {
		if isBal {
			cc.powerBalBtn.AddCSSClass("active")
		} else {
			cc.powerBalBtn.RemoveCSSClass("active")
		}
	}
	if cc.powerSaverBtn != nil {
		if isSaver {
			cc.powerSaverBtn.AddCSSClass("active")
		} else {
			cc.powerSaverBtn.RemoveCSSClass("active")
		}
	}
}

func (cc *ControlCenter) updateInputUI(vol float64, muted bool) {
	if cc.inputBinding != nil {
		pct := int(math.Round(vol))
		cc.inputBinding.SetValue(pct, muted)
	}
}

func (cc *ControlCenter) renderAudioStreams(streams []AudioStream) {
	if cc.audioStreamsListBox == nil {
		return
	}

	if len(streams) == 0 {
		for id, sr := range cc.streamRows {
			if sr.cancelWorker != nil {
				sr.cancelWorker()
			}
			cc.audioStreamsListBox.Remove(sr.listBoxRow)
			delete(cc.streamRows, id)
		}
		if cc.audioEmptyLabel != nil {
			cc.audioEmptyLabel.SetVisible(true)
		}
		if cc.audioStreamsScrolledWin != nil {
			cc.audioStreamsScrolledWin.SetVisible(false)
		}
		return
	}

	if cc.audioEmptyLabel != nil {
		cc.audioEmptyLabel.SetVisible(false)
	}
	if cc.audioStreamsScrolledWin != nil {
		cc.audioStreamsScrolledWin.SetVisible(true)
	}

	activeIDs := make(map[int]bool)
	for _, stream := range streams {
		activeIDs[stream.ID] = true
		st := stream

		if sr, exists := cc.streamRows[st.ID]; exists {
			sr.name.SetText(st.AppName)
			if st.MediaName != "" && st.MediaName != st.AppName {
				sr.media.SetText(st.MediaName)
				sr.media.SetVisible(true)
			} else {
				sr.media.SetVisible(false)
			}
			sr.icon.SetFromIconName(st.IconName)

			sr.binding.SetValue(st.Volume, st.Muted)
		} else {
			builder := gtk.NewBuilderFromString(ui.AudioStreamItem)
			rowBox := builder.GetObject("audio_stream_box").Cast().(*gtk.Box)
			iconImg := builder.GetObject("audio_stream_icon").Cast().(*gtk.Image)
			nameLbl := builder.GetObject("audio_stream_name").Cast().(*gtk.Label)
			mediaLbl := builder.GetObject("audio_stream_media").Cast().(*gtk.Label)
			muteBtn := builder.GetObject("audio_stream_mute_btn").Cast().(*gtk.Button)
			muteIcon := builder.GetObject("audio_stream_mute_icon").Cast().(*gtk.Image)
			scale := builder.GetObject("audio_stream_scale").Cast().(*gtk.Scale)
			volLbl := builder.GetObject("audio_stream_vol_label").Cast().(*gtk.Label)

			iconImg.SetFromIconName(st.IconName)
			nameLbl.SetText(st.AppName)
			if st.MediaName != "" && st.MediaName != st.AppName {
				mediaLbl.SetText(st.MediaName)
				mediaLbl.SetVisible(true)
			} else {
				mediaLbl.SetVisible(false)
			}

			streamID := st.ID
			binding := NewSliderBinding(SliderConfig{
				Scale:  scale,
				Label:  volLbl,
				Button: muteBtn,
				Icon:   muteIcon,
				OnApply: func(pct int) {
					cc.audioCtrl.SetSinkInputVolume(streamID, pct)
				},
				OnMute: func() {
					cc.audioCtrl.ToggleSinkInputMute(streamID)
				},
				GetIcon: func(pct int, muted bool) string {
					if muted {
						muteBtn.AddCSSClass("muted")
						return "audio-volume-muted-symbolic"
					}
					muteBtn.RemoveCSSClass("muted")
					return "audio-volume-high-symbolic"
				},
			})

			binding.SetValue(st.Volume, st.Muted)

			streamCtx, cancelWorker := context.WithCancel(context.Background())
			binding.StartWorker(streamCtx)

			listBoxRow := gtk.NewListBoxRow()
			listBoxRow.SetChild(rowBox)
			listBoxRow.SetSelectable(false)
			listBoxRow.SetActivatable(false)
			cc.audioStreamsListBox.Append(listBoxRow)

			cc.streamRows[streamID] = &streamRow{
				binding:      binding,
				cancelWorker: cancelWorker,
				listBoxRow:   listBoxRow,
				rowBox:       rowBox,
				icon:         iconImg,
				name:         nameLbl,
				media:        mediaLbl,
				muteBtn:      muteBtn,
				muteIcon:     muteIcon,
			}
		}
	}

	for id, sr := range cc.streamRows {
		if !activeIDs[id] {
			if sr.cancelWorker != nil {
				sr.cancelWorker()
			}
			cc.audioStreamsListBox.Remove(sr.listBoxRow)
			delete(cc.streamRows, id)
		}
	}
}

func (cc *ControlCenter) Close() {
	glib.IdleAdd(func() {
		cc.mu.Lock()
		cc.visible = false
		cc.wifiPassSSID = ""
		cc.wifiLastSig = ""
		cc.mu.Unlock()

		if cc.contentStack != nil {
			cc.contentStack.SetVisibleChildName("main")
		}

		cc.window.SetVisible(false)
	})
}

func (cc *ControlCenter) Toggle() {
	cc.mu.Lock()
	vis := cc.visible
	cc.mu.Unlock()

	if vis {
		cc.Close()
	} else {
		cc.Open()
	}
}

func (cc *ControlCenter) IsVisible() bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return cc.visible
}

func (cc *ControlCenter) updateMprisUI(active *mpris.PlayerState) {
	if cc.mprisDestroyed {
		return
	}
	if active == nil || active.Status == mpris.PlaybackStopped {
		cc.mprisCard.SetVisible(false)
		return
	}

	cc.mprisCard.SetVisible(true)

	title := active.Title
	if strings.TrimSpace(title) == "" {
		title = "Unknown Track"
	}
	cc.mprisTitle.SetText(title)

	artist := active.Artist
	if strings.TrimSpace(artist) == "" {
		if active.Album != "" {
			artist = active.Album
		} else {
			artist = "Unknown Artist"
		}
	}
	cc.mprisArtist.SetText(artist)
	cc.mprisPlayerName.SetText(active.Identity)

	// Art priority: 1. Cover Art, 2. App Icon, 3. Fallback
	cc.updateMprisArt(active)

	if active.Status == mpris.PlaybackPlaying {
		cc.mprisPlayBtn.SetIconName("media-playback-pause-symbolic")
	} else {
		cc.mprisPlayBtn.SetIconName("media-playback-start-symbolic")
	}

	cc.mprisPrevBtn.SetSensitive(active.CanGoPrevious)
	cc.mprisNextBtn.SetSensitive(active.CanGoNext)
	cc.mprisPlayBtn.SetSensitive(active.CanPlay || active.CanPause || active.CanControl)
}

func loadCroppedTexture(path string, targetSize int) *gdk.Texture {
	pb, err := gdkpixbuf.NewPixbufFromFile(path)
	if err != nil || pb == nil {
		return nil
	}
	origW := pb.Width()
	origH := pb.Height()
	if origW <= 0 || origH <= 0 {
		return nil
	}

	// Compute scale to cover targetSize x targetSize preserving aspect ratio
	scale := float64(targetSize) / float64(origW)
	if sH := float64(targetSize) / float64(origH); sH > scale {
		scale = sH
	}

	newW := int(math.Ceil(float64(origW) * scale))
	newH := int(math.Ceil(float64(origH) * scale))

	scaled := pb.ScaleSimple(newW, newH, gdkpixbuf.InterpBilinear)
	if scaled == nil {
		return nil
	}

	cropX := (newW - targetSize) / 2
	cropY := (newH - targetSize) / 2
	if cropX < 0 {
		cropX = 0
	}
	if cropY < 0 {
		cropY = 0
	}
	if cropX+targetSize > scaled.Width() {
		cropX = scaled.Width() - targetSize
	}
	if cropY+targetSize > scaled.Height() {
		cropY = scaled.Height() - targetSize
	}
	if cropX < 0 {
		cropX = 0
	}
	if cropY < 0 {
		cropY = 0
	}

	actualW := targetSize
	if actualW > scaled.Width()-cropX {
		actualW = scaled.Width() - cropX
	}
	actualH := targetSize
	if actualH > scaled.Height()-cropY {
		actualH = scaled.Height() - cropY
	}

	cropped := scaled.NewSubpixbuf(cropX, cropY, actualW, actualH)
	if cropped == nil {
		return gdk.NewTextureForPixbuf(scaled)
	}
	return gdk.NewTextureForPixbuf(cropped)
}

func (cc *ControlCenter) updateMprisArt(active *mpris.PlayerState) {
	const artSize = 64

	// 1. Cover Art: local file or cached remote file
	artPath := active.LocalArtPath
	if artPath == "" && active.ArtURL != "" {
		raw := active.ArtURL
		if strings.HasPrefix(raw, "file://") {
			artPath = strings.TrimPrefix(raw, "file://")
		} else if strings.HasPrefix(raw, "/") {
			artPath = raw
		}
	}

	if artPath != "" {
		if _, err := os.Stat(artPath); err == nil {
			if tex := cc.croppedTexture(artPath, artSize); tex != nil {
				cc.mprisArt.SetPixelSize(artSize)
				cc.mprisArt.SetFromPaintable(tex)
				return
			}
		}
	}

	// 2. App Icon from active player identity & bus name
	display := gdk.DisplayGetDefault()
	if display != nil {
		theme := gtk.IconThemeGetForDisplay(display)
		if theme != nil {
			// Candidate names derived from the player identity and bus name
			// only; fall through to the generic icon when nothing matches.
			candidates := make([]string, 0, 4)
			if ident := strings.TrimSpace(active.Identity); ident != "" {
				candidates = append(candidates, strings.ToLower(ident), ident)
			}
			busShort := strings.TrimPrefix(active.BusName, "org.mpris.MediaPlayer2.")
			if busShort != "" && busShort != active.Identity {
				candidates = append(candidates, strings.ToLower(busShort), busShort)
			}

			for _, c := range candidates {
				if c == "" {
					continue
				}
				if theme.HasIcon(c) {
					cc.mprisArt.SetPixelSize(36)
					cc.mprisArt.SetFromIconName(c)
					return
				}
				// Also try splitting dot identifiers
				if strings.Contains(c, ".") {
					parts := strings.Split(c, ".")
					for j := len(parts) - 1; j >= 0; j-- {
						p := strings.ToLower(parts[j])
						if p != "desktop" && p != "exe" && theme.HasIcon(p) {
							cc.mprisArt.SetPixelSize(36)
							cc.mprisArt.SetFromIconName(p)
							return
						}
					}
				}
			}
		}
	}

	// 3. Fallback generic audio icon
	cc.mprisArt.SetPixelSize(32)
	cc.mprisArt.SetFromIconName("audio-x-generic-symbolic")
}

// croppedTexture loads and center-crops an image to targetSize, caching the
// result per path so repeated state changes don't re-decode the file.
func (cc *ControlCenter) croppedTexture(path string, targetSize int) *gdk.Texture {
	if tex, ok := cc.mprisTexCache[path]; ok {
		return tex
	}
	tex := loadCroppedTexture(path, targetSize)
	if tex != nil {
		// Drop any previous path so at most one cached texture lingers.
		if cc.mprisTexPath != "" && cc.mprisTexPath != path {
			delete(cc.mprisTexCache, cc.mprisTexPath)
		}
		cc.mprisTexCache[path] = tex
		cc.mprisTexPath = path
	}
	return tex
}

func (cc *ControlCenter) applyStyle(styleName, styleClass string) {
	if cc.card == nil {
		return
	}
	for _, s := range []string{"control-center-style-cards", "control-center-style-compact", "control-center-style-default"} {
		cc.card.RemoveCSSClass(s)
	}
	cc.card.AddCSSClass(styleClass)

	isCompact := styleName == "compact"
	if isCompact {
		cc.card.SetSizeRequest(320, -1)
	} else {
		cc.card.SetSizeRequest(390, -1)
	}

	// Always keep the subpanel arrow buttons visible in both styles
	if cc.wifiArrowButton != nil {
		cc.wifiArrowButton.SetVisible(true)
	}
	if cc.bluetoothArrowButton != nil {
		cc.bluetoothArrowButton.SetVisible(true)
	}
	if cc.powerArrowButton != nil {
		cc.powerArrowButton.SetVisible(true)
	}
	if cc.volumeArrowButton != nil {
		cc.volumeArrowButton.SetVisible(true)
	}

	// In compact style, collapse the extra subtitle labels to keep tiles slim & clean
	if cc.wifiStatus != nil {
		cc.wifiStatus.SetVisible(!isCompact)
	}
	if cc.bluetoothStatus != nil {
		cc.bluetoothStatus.SetVisible(!isCompact)
	}
	if cc.powerStatus != nil {
		cc.powerStatus.SetVisible(!isCompact)
	}
	if cc.dndStatus != nil {
		cc.dndStatus.SetVisible(!isCompact)
	}
}

func (cc *ControlCenter) UpdateConfig(cfg config.ControlCenterConfig) {
	if cc == nil {
		return
	}
	styleName, styleClass := style.Resolve(style.ControlCenter, cfg.Style)
	glib.IdleAdd(func() {
		cc.applyStyle(styleName, styleClass)
	})
}

func (cc *ControlCenter) Destroy() {
	cc.mu.Lock()
	cc.mprisDestroyed = true
	cc.mu.Unlock()
	cc.Close()
	if cc.mprisCtrl != nil {
		_ = cc.mprisCtrl.Close()
	}
	glib.IdleAdd(func() {
		for _, sr := range cc.streamRows {
			if sr.cancelWorker != nil {
				sr.cancelWorker()
			}
		}
		cc.streamRows = nil
		if cc.window != nil {
			if cc.window.Realized() {
				cc.window.Destroy()
			}
			cc.window = nil
		}
	})
}
