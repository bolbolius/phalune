package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"phalune/internal/config"
	"phalune/ui"

	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Urgency byte

const (
	UrgencyLow      Urgency = 0
	UrgencyNormal   Urgency = 1
	UrgencyCritical Urgency = 2
)

const (
	CloseReasonExpired         uint32 = 1
	CloseReasonDismissedByUser uint32 = 2
	CloseReasonClosedByCall    uint32 = 3
	CloseReasonUndefined       uint32 = 4
)

type Action struct {
	Key   string
	Label string
}

type Notification struct {
	ID            uint32
	AppName       string
	Summary       string
	Body          string
	Icon          string
	Actions       []Action
	Hints         map[string]any
	Urgency       Urgency
	ExpireTimeout time.Duration
}

type notificationItem struct {
	id    uint32
	card  *gtk.Box
	timer *time.Timer
}

type Manager struct {
	window    *gtk.Window
	container *gtk.Box

	timeoutLow     time.Duration
	timeoutNormal  time.Duration
	criticalSticky bool

	mu     sync.Mutex
	nextID uint32
	items  map[uint32]*notificationItem
	dnd    bool
	store  *Store

	onAction func(id uint32, actionKey string)
	onClose  func(id uint32, reason uint32)
	onReply  func(id uint32, text string)
}

func New(app *gtk.Application, cfg config.NotificationsConfig) (*Manager, error) {
	win := gtk.NewWindow()
	win.SetApplication(app)
	win.SetTitle("phalune-notify")
	win.SetDecorated(false)
	win.AddCSSClass("notify-window")

	if err := ConfigureNotifySurface(win, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure notification surface: %w", err)
	}

	builder := gtk.NewBuilderFromString(ui.Notify)
	container := builder.GetObject("notify_container").Cast().(*gtk.Box)

	win.SetChild(container)

	timeoutLow := cfg.TimeoutLow.Duration
	if timeoutLow <= 0 {
		timeoutLow = 3 * time.Second
	}
	timeoutNormal := cfg.TimeoutNormal.Duration
	if timeoutNormal <= 0 {
		timeoutNormal = 5 * time.Second
	}

	return &Manager{
		window:         win,
		container:      container,
		timeoutLow:     timeoutLow,
		timeoutNormal:  timeoutNormal,
		criticalSticky: cfg.CriticalSticky,
		nextID:         1,
		items:          make(map[uint32]*notificationItem),
		store:          NewStore(),
	}, nil
}

func (m *Manager) UpdateConfig(cfg config.NotificationsConfig) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	timeoutLow := cfg.TimeoutLow.Duration
	if timeoutLow <= 0 {
		timeoutLow = 3 * time.Second
	}
	timeoutNormal := cfg.TimeoutNormal.Duration
	if timeoutNormal <= 0 {
		timeoutNormal = 5 * time.Second
	}
	m.timeoutLow = timeoutLow
	m.timeoutNormal = timeoutNormal
	m.criticalSticky = cfg.CriticalSticky
	win := m.window
	m.mu.Unlock()

	if win != nil {
		return ConfigureNotifySurface(win, cfg)
	}
	return nil
}

func (m *Manager) SetOnAction(fn func(id uint32, actionKey string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAction = fn
}

func (m *Manager) SetOnClose(fn func(id uint32, reason uint32)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onClose = fn
}

func (m *Manager) SetDND(enabled bool) {
	m.mu.Lock()
	m.dnd = enabled
	m.mu.Unlock()
}

func (m *Manager) IsDND() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dnd
}

func (m *Manager) Show(n Notification) uint32 {
	m.mu.Lock()
	id := n.ID
	if id == 0 {
		id = m.nextID
		m.nextID++
	} else if id >= m.nextID {
		m.nextID = id + 1
	}

	n.ID = id
	if m.store != nil {
		m.store.Add(n)
	}

	if m.dnd && n.Urgency != UrgencyCritical {
		m.mu.Unlock()
		return id
	}

	if existing, ok := m.items[id]; ok && existing.timer != nil {
		existing.timer.Stop()
	}
	m.mu.Unlock()

	timeout := n.ExpireTimeout
	if timeout <= 0 {
		if n.Urgency == UrgencyCritical {
			if m.criticalSticky {
				timeout = 0
			} else {
				timeout = m.timeoutNormal
			}
		} else if n.Urgency == UrgencyLow {
			timeout = m.timeoutLow
		} else {
			timeout = m.timeoutNormal
		}
	}
	if n.Icon == "" {
		if path, ok := n.Hints["image-path"].(string); ok && path != "" {
			n.Icon = path
		} else {
			n.Icon = "dialog-information"
		}
	}
	if n.AppName == "" {
		n.AppName = "phalune"
	}

	glib.IdleAdd(func() {
		m.mu.Lock()
		if oldItem, ok := m.items[id]; ok && oldItem.card != nil {
			m.container.Remove(oldItem.card)
		}
		m.mu.Unlock()

		cardBuilder := gtk.NewBuilderFromString(ui.NotifyCard)
		card := cardBuilder.GetObject("notify_card").Cast().(*gtk.Box)
		icon := cardBuilder.GetObject("card_icon").Cast().(*gtk.Image)
		appLabel := cardBuilder.GetObject("app_label").Cast().(*gtk.Label)
		closeBtn := cardBuilder.GetObject("close_button").Cast().(*gtk.Button)
		titleLabel := cardBuilder.GetObject("title_label").Cast().(*gtk.Label)
		bodyLabel := cardBuilder.GetObject("body_label").Cast().(*gtk.Label)
		actionsBox := cardBuilder.GetObject("actions_box").Cast().(*gtk.Box)
		replyBox := cardBuilder.GetObject("reply_box").Cast().(*gtk.Box)
		replyEntry := cardBuilder.GetObject("reply_entry").Cast().(*gtk.Entry)
		replyBtn := cardBuilder.GetObject("reply_button").Cast().(*gtk.Button)

		if strings.HasPrefix(n.Icon, "/") || strings.HasPrefix(n.Icon, "file://") {
			icon.SetFromFile(strings.TrimPrefix(n.Icon, "file://"))
		} else {
			icon.SetFromIconName(n.Icon)
		}

		appLabel.SetText(n.AppName)
		titleLabel.SetText(n.Summary)

		if n.Body != "" {
			if strings.Contains(n.Body, "<") && strings.Contains(n.Body, ">") {
				bodyLabel.SetMarkup(n.Body)
			} else {
				bodyLabel.SetText(n.Body)
			}
			bodyLabel.SetVisible(true)
		} else {
			bodyLabel.SetVisible(false)
		}

		switch n.Urgency {
		case UrgencyLow:
			card.AddCSSClass("urgency-low")
		case UrgencyCritical:
			card.AddCSSClass("urgency-critical")
		default:
			card.AddCSSClass("urgency-normal")
		}

		closeBtn.ConnectClicked(func() {
			m.dismiss(id, CloseReasonDismissedByUser)
		})

		var hasDefaultAction bool
		var nonDefaultActions []Action
		hasReply := false
		if hr, ok := n.Hints["has-reply"].(bool); ok && hr {
			hasReply = true
		}

		for _, act := range n.Actions {
			if act.Key == "default" {
				hasDefaultAction = true
			} else if act.Key == "inline-reply" || act.Key == "reply" {
				hasReply = true
			} else {
				nonDefaultActions = append(nonDefaultActions, act)
			}
		}

		click := gtk.NewGestureClick()
		click.ConnectReleased(func(count int, x, y float64) {
			if hasDefaultAction {
				m.invokeAction(id, "default")
			}
			m.dismiss(id, CloseReasonDismissedByUser)
		})
		card.AddController(click)

		if len(nonDefaultActions) > 0 {
			for _, act := range nonDefaultActions {
				btn := gtk.NewButtonWithLabel(act.Label)
				btn.AddCSSClass("notify-action-btn")
				actionKey := act.Key
				btn.ConnectClicked(func() {
					m.invokeAction(id, actionKey)
					m.dismiss(id, CloseReasonDismissedByUser)
				})
				actionsBox.Append(btn)
			}
			actionsBox.SetVisible(true)
		}

		if hasReply {
			replyBox.SetVisible(true)
			sendReply := func() {
				text := replyEntry.Text()
				if text != "" {
					m.invokeReply(id, text)
					m.dismiss(id, CloseReasonDismissedByUser)
				}
			}
			replyBtn.ConnectClicked(sendReply)
			replyEntry.ConnectActivate(sendReply)
		}

		item := &notificationItem{
			id:   id,
			card: card,
		}

		if timeout > 0 {
			item.timer = time.AfterFunc(timeout, func() {
				glib.IdleAdd(func() {
					m.dismiss(id, CloseReasonExpired)
				})
			})
		}

		m.mu.Lock()
		m.items[id] = item
		m.mu.Unlock()

		m.container.Append(card)
		m.window.Present()
	})

	return id
}

func (m *Manager) Notify(n Notification) {
	m.Show(n)
}

func (m *Manager) CloseNotification(id uint32) {
	m.dismiss(id, CloseReasonClosedByCall)
}

func (m *Manager) dismiss(id uint32, reason uint32) {
	m.mu.Lock()
	item, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if item.timer != nil {
		item.timer.Stop()
	}
	delete(m.items, id)
	remaining := len(m.items)
	onClose := m.onClose
	m.mu.Unlock()

	glib.IdleAdd(func() {
		if item.card != nil {
			m.container.Remove(item.card)
		}
		if remaining == 0 {
			m.window.SetVisible(false)
		}
	})

	if onClose != nil {
		onClose(id, reason)
	}
}

func (m *Manager) invokeAction(id uint32, actionKey string) {
	m.mu.Lock()
	onAction := m.onAction
	m.mu.Unlock()

	if onAction != nil {
		onAction(id, actionKey)
	}
}

func (m *Manager) SetOnReply(fn func(id uint32, text string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReply = fn
}

func (m *Manager) invokeReply(id uint32, text string) {
	m.mu.Lock()
	onReply := m.onReply
	m.mu.Unlock()

	if onReply != nil {
		onReply(id, text)
	}
}

func (m *Manager) Store() *Store {
	if m == nil {
		return nil
	}
	return m.store
}

func (m *Manager) InvokeAction(id uint32, actionKey string) {
	m.invokeAction(id, actionKey)
}

func (m *Manager) InvokeReply(id uint32, text string) {
	m.invokeReply(id, text)
}

func (m *Manager) Dismiss(id uint32, reason uint32) {
	m.dismiss(id, reason)
}

func (m *Manager) ClearAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	ids := make([]uint32, 0, len(m.items))
	for id := range m.items {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	for _, id := range ids {
		m.dismiss(id, CloseReasonDismissedByUser)
	}
	if m.store != nil {
		m.store.Clear()
	}
}

func (m *Manager) Destroy() {
	m.mu.Lock()
	for _, item := range m.items {
		if item.timer != nil {
			item.timer.Stop()
		}
	}
	m.items = make(map[uint32]*notificationItem)
	m.mu.Unlock()

	if m.window != nil && m.window.Realized() {
		m.window.Destroy()
	}
}
