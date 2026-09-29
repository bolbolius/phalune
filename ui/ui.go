package ui

import _ "embed"

// Bar
//
//go:embed bar/bar.ui
var Bar string

//go:embed bar/bar_section.ui
var BarSection string

// Launcher
//
//go:embed launcher/launcher.ui
var Launcher string

//go:embed launcher/launcher_item.ui
var LauncherItem string

// Clipboard
//
//go:embed clipboard/clipboard.ui
var Clipboard string

//go:embed clipboard/clipboard_item.ui
var ClipboardItem string

// Control Center
//
//go:embed controlcenter/controlcenter.ui
var ControlCenter string

//go:embed controlcenter/wifi_item.ui
var WifiItem string

//go:embed controlcenter/bluetooth_item.ui
var BluetoothItem string

//go:embed controlcenter/audio_stream_item.ui
var AudioStreamItem string

// Notify
//
//go:embed notify/notify.ui
var Notify string

//go:embed notify/notify_card.ui
var NotifyCard string

// OSD
//
//go:embed osd/osd.ui
var OSD string

// Notification Center
//
//go:embed notificationcenter/notificationcenter.ui
var NotificationCenter string

//go:embed notificationcenter/notification_item.ui
var NotificationHistoryItem string

// Widgets
//
//go:embed widget/clock.ui
var Clock string

//go:embed widget/workspaces.ui
var Workspaces string

//go:embed widget/workspace_button.ui
var WorkspaceButton string

//go:embed widget/audio.ui
var Audio string

//go:embed widget/battery.ui
var Battery string

//go:embed widget/tray.ui
var Tray string

//go:embed widget/tray_item.ui
var TrayItem string

//go:embed widget/bluetooth.ui
var Bluetooth string

//go:embed widget/wifi.ui
var Wifi string

//go:embed widget/keyboard.ui
var Keyboard string

//go:embed widget/power.ui
var Power string

//go:embed widget/clipboard.ui
var ClipboardWidget string

//go:embed widget/notifications.ui
var NotificationsWidget string

//go:embed widget/privacy.ui
var PrivacyWidget string

// LockScreen
//
//go:embed lockscreen/lockscreen.ui
var LockScreen string

// Power Menu
//
//go:embed powermenu/powermenu.ui
var PowerMenu string

// Window Switcher
//
//go:embed windowswitcher/windowswitcher.ui
var WindowSwitcher string

//go:embed windowswitcher/window_card.ui
var WindowSwitcherCard string
