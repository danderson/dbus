// Package notifications provides an interface to the Freedesktop
// notifications API.
//
// This corresponds to the org.freedesktop.Notifications service on
// the session bus.
package notifications

import (
	"context"

	"github.com/danderson/dbus"
)

type Notification struct{ iface dbus.Interface }

// New returns an interface to the session's notification service.
func New(conn *dbus.Conn) Notification {
	obj := conn.Peer("org.freedesktop.Notifications").Object("/org/freedesktop/Notifications")
	return Interface(obj)
}

// Interface returns a Notification on the given object.
func Interface(obj dbus.Object) Notification {
	return Notification{
		iface: obj.Interface("org.freedesktop.Notifications"),
	}
}

func (iface Notification) CloseNotification(ctx context.Context, id uint32) error {
	err := iface.iface.Call(ctx, "CloseNotification", id, nil)
	return err
}

// Capabilities supported by various DEs
//
// Actions supported by Gnome
// ==========================
// actions
// body
// body-markup
// icon-static
// persistence
// sound
//
// Actions supported by KDE
// ========================
// actions
// body
// body-hyperlinks
// body-images
// body-markup
// icon-static
// inhibitions
// inline-reply
// persistence
// x-kde-display-appname
// x-kde-origin-name
// x-kde-urls
//
// Not mentioned in standards
// ==========================
// inhibitions
// inline-reply
//
// In standard but nobody implements?
// ==================================
// action-icons
// icon-multi

// Capabilities enumerates the optional capabilities of a notification
// service.
type Capabilities struct {
	// Actions reports whether notifications can have actions attached
	// to them. Actions trigger a signal back to the notification's
	// sender when interacted with.
	Actions bool
	// ActionIcons reports notification actions can use icons to
	// describe actions instead of text.
	ActionIcons bool
	// Body reports whether notifications can have a body, in addition
	// to a short title.
	//
	// Most notification services support bodies, but clients should
	// not assume that all do.
	Body bool
	// BodyLinks reports whether notification bodies can include
	// hyperlinks.
	BodyLinks bool
	// BodyImages reports whether notification bodies can include
	// images.
	BodyImages bool
	// BodyMarkup reports whether notification bodies can contain
	// notification markup, a small subset of HTML.
	BodyMarkup bool
	// Icon reports whether notifications can have an icon.
	Icon bool
	// IconAnimation reports whether the notification icon can be
	// multiple frames of animation, or just a single static frame.
	IconAnimation bool
	// Persistence reports whether notifications can be
	// persistent. Persistent notifications remain on screen until
	// explicitly dismissed by the user.
	Persistence bool
	// Sound reports whether notifications can play a sound.
	Sound bool

	// Inhibitions reports whether the notification service supports
	// the Inhibit call, for controlled suppression of notifications.
	//
	// Inhibitions is a KDE-only extension to the notifications API.
	Inhibitions bool
	// InlineReply reports whether notifications can prompt for text
	// reply within the notification.
	//
	// InlineReply is a KDE-only extension to the notifications API.
	InlineReply bool
	// ContextURLs reports whether notifications can include URL
	// hints, to enrich the notification's interaction options. For
	// example, a file:// URL adds a context menu to interact with the
	// file, whereas https:// URLs show a site preview.
	//
	// ContextURLs is a KDE-only extension to the notifications API.
	ContextURLs bool
	// DisplayAppName reports whether notifications can show a pretty
	// name for the sending application.
	//
	// DisplayAppName is a KDE-only extension to the notifications API.
	DisplayAppName bool
	// DisplayOriginName reports whether notifications can show an
	// additional "origin" for notification, e.g. a website domain or
	// a message's sender in chat apps.
	//
	// DisplayOriginName is a KDE-only extension to the notifications
	// API.
	DisplayOriginName bool

	// Unknown collects the capability strings that aren't known to
	// this package.
	Unknown []string
}

// Capabilities reports the capabilities of the notification service.
func (iface Notification) Capabilities(ctx context.Context) (caps Capabilities, err error) {
	var cs []string
	if err := iface.iface.Call(ctx, "GetCapabilities", nil, &cs); err != nil {
		return Capabilities{}, err
	}
	for _, c := range cs {
		switch c {
		case "actions":
			caps.Actions = true
		case "action-icons":
			caps.ActionIcons = true
		case "body":
			caps.Body = true
		case "body-hyperlinks":
			caps.BodyLinks = true
		case "body-images":
			caps.BodyImages = true
		case "body-markup":
			caps.BodyMarkup = true
		case "icon-static":
			caps.Icon = true
		case "icon-multi":
			caps.Icon = true
			caps.IconAnimation = true
		case "persistence":
			caps.Persistence = true
		case "sound":
			caps.Sound = true

		case "inhibitions":
			caps.Inhibitions = true
		case "inline-reply":
			caps.InlineReply = true
		case "x-kde-display-appname":
			caps.DisplayAppName = true
		case "x-kde-origin-name":
			caps.DisplayOriginName = true
		case "x-kde-urls":
			caps.ContextURLs = true

		default:
			caps.Unknown = append(caps.Unknown, c)
		}
	}
	return caps, nil
}

type GetServerInformationResponse struct {
	Name        string
	Vendor      string
	Version     string
	SpecVersion string
}

func (iface Notification) GetServerInformation(ctx context.Context) (resp GetServerInformationResponse, err error) {
	err = iface.iface.Call(ctx, "GetServerInformation", nil, &resp)
	return resp, err
}

func (iface Notification) Inhibit(ctx context.Context, desktopEntry string, reason string, hints map[string]interface{}) (arg0 uint32, err error) {
	req := struct {
		DesktopEntry string
		Reason       string
		Hints        map[string]interface{}
	}{
		DesktopEntry: desktopEntry,
		Reason:       reason,
		Hints:        hints,
	}
	err = iface.iface.Call(ctx, "Inhibit", req, &arg0)
	return arg0, err
}

type NotifyRequest struct {
	AppName    string
	ReplacesID uint32
	AppIcon    string
	Summary    string
	Body       string
	Actions    []string
	Hints      map[string]interface{}
	Timeout    int32
}

func (iface Notification) Notify(ctx context.Context, req NotifyRequest) (arg0 uint32, err error) {
	err = iface.iface.Call(ctx, "Notify", req, &arg0)
	return arg0, err
}

func (iface Notification) UnInhibit(ctx context.Context, arg0 uint32) error {
	err := iface.iface.Call(ctx, "UnInhibit", arg0, nil)
	return err
}

// Inhibited returns the value of the property "Inhibited".
func (iface Notification) Inhibited(ctx context.Context) (bool, error) {
	var ret bool
	err := iface.iface.GetProperty(ctx, "Inhibited", &ret)
	return ret, err
}

// InhibitedChanged signals that the value of property "Inhibited" has changed.
type InhibitedChanged bool

// ActionInvoked implements the signal org.freedesktop.Notifications.ActionInvoked.
type ActionInvoked struct {
	Id        uint32
	ActionKey string
}

// ActivationToken implements the signal org.freedesktop.Notifications.ActivationToken.
type ActivationToken struct {
	Id              uint32
	ActivationToken string
}

// NotificationClosed implements the signal org.freedesktop.Notifications.NotificationClosed.
type NotificationClosed struct {
	Id     uint32
	Reason uint32
}

// NotificationReplied implements the signal org.freedesktop.Notifications.NotificationReplied.
type NotificationReplied struct {
	Id   uint32
	Text string
}

func init() {
	dbus.RegisterPropertyChangeType[InhibitedChanged]("org.freedesktop.Notifications", "Inhibited")
	dbus.RegisterSignalType[ActionInvoked]("org.freedesktop.Notifications", "ActionInvoked")
	dbus.RegisterSignalType[ActivationToken]("org.freedesktop.Notifications", "ActivationToken")
	dbus.RegisterSignalType[NotificationClosed]("org.freedesktop.Notifications", "NotificationClosed")
	dbus.RegisterSignalType[NotificationReplied]("org.freedesktop.Notifications", "NotificationReplied")
}
