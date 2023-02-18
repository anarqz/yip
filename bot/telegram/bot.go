package telegram

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alcmoraes/yip/routers"
	"github.com/spf13/viper"
	tele "gopkg.in/telebot.v3"
)

type TelegramBot struct {
	bot       *tele.Bot
	router    routers.Router
	authGroup map[int64]bool
	nameCache map[string]string
}

func (t *TelegramBot) Start() {
	pref := tele.Settings{
		Token:  viper.GetString("telegram.token"),
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)

	t.bot = b

	if err != nil {
		log.Fatal(err)
		return
	}

	menu := &tele.ReplyMarkup{ForceReply: true, ResizeKeyboard: true}

	btnReauthenticateRouter := menu.Text("🔐 Reauth Router")
	btnWhitelistMac := menu.Text("👍 Allow MAC")
	btnBlacklistMac := menu.Text("👎 Deny MAC")
	btnClearBlacklist := menu.Text("🧹 Clear Blacklist")

	menu.Reply(
		menu.Row(btnReauthenticateRouter),
		menu.Row(btnBlacklistMac, btnWhitelistMac),
		menu.Row(btnClearBlacklist),
	)

	// Login to bot
	b.Handle("/start", func(c tele.Context) error {
		return c.Send("Hello!\nUse: \n\n/login PASSWORD\n\nto authenticate before using it", menu)
	})

	b.Handle("/login", t.Login)
	b.Handle(&btnReauthenticateRouter, t.ReauthenticateRouter, t.LockMiddleware)
	b.Handle(&btnWhitelistMac, t.WhitelistMac, t.LockMiddleware)
	b.Handle(&btnBlacklistMac, t.BlacklistMac, t.LockMiddleware)
	b.Handle(&btnClearBlacklist, t.ClearBlacklist, t.LockMiddleware)

	b.Handle(tele.OnCallback, t.OnCallback, t.LockMiddleware)

	b.Start()
}

func (t *TelegramBot) LockMiddleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		if c.Callback() != nil {
			defer c.Respond()
		}
		if t.authGroup[c.Sender().ID] {
			return next(c) // continue execution chain
		}
		return c.Send("You are not authenticated.\nPlease use\n\n/login PASSWORD\n\nto authenticate")
	}
}

func (t *TelegramBot) Login(c tele.Context) error {
	arguments := c.Args()
	if len(arguments) == 0 {
		return c.Send("Please use\n\n/login PASSWORD\n\nto authenticate")
	}
	if arguments[0] == viper.GetString("telegram.password") {
		t.authGroup[c.Sender().ID] = true
		return c.Send("Login successful")
	} else {
		return c.Send("Wrong password")
	}
}

func (t *TelegramBot) ReauthenticateRouter(c tele.Context) error {
	if err := t.router.RefreshToken(); err != nil {
		return c.Send("Failed to reauthenticate the router")
	}
	return c.Send("Router reauthenticated successfully")
}

func (t *TelegramBot) BlacklistMac(c tele.Context) error {
	devices, err := t.router.ListDevices()
	if err != nil {
		return c.Send("Failed to list devices")
	}

	menu := &tele.ReplyMarkup{}
	options := make([]tele.Row, 0)

	for _, d := range devices {
		t.nameCache[strings.ToUpper(d.MacAddress)] = d.Name
		btn := menu.Data(fmt.Sprintf("%s (%s)", d.MacAddress, d.Name), "/block", d.MacAddress)
		options = append(options, menu.Row(btn))
	}
	menu.Inline(options...)

	return c.Send("Choose the device to block:", menu)
}

func (t *TelegramBot) WhitelistMac(c tele.Context) error {
	devices, err := t.router.GetFilteredDevices()
	if err != nil {
		return c.Send("Failed to show blacklist")
	}
	if len(devices) == 0 {
		return c.Send("No devices are blocked")
	}

	menu := &tele.ReplyMarkup{}
	options := make([]tele.Row, 0)

	for _, d := range devices {
		cachedName := t.nameCache[strings.ToUpper(d.MacAddress)]
		if cachedName != "" {
			d.Name = cachedName
		}
		btn := menu.Data(fmt.Sprintf("%s (%s)", d.MacAddress, d.Name), "/unblock", d.MacAddress)
		options = append(options, menu.Row(btn))
	}
	menu.Inline(options...)

	return c.Send("Choose the device to unblock:", menu)
}

func (t *TelegramBot) ClearBlacklist(c tele.Context) error {
	if err := t.router.ClearMacFilters(); err != nil {
		return c.Send("Failed to clear MAC filters")
	}
	return c.Send("All devices are allowed now")
}

func (t *TelegramBot) OnCallback(c tele.Context) error {

	command := strings.TrimSpace(c.Args()[0])

	switch command {
	case "/unblock":
		if err := t.router.UnfilterDeviceByMac(c.Args()[1]); err != nil {
			return c.Send("Failed to unblock the device")
		} else {
			return c.Send(fmt.Sprintf("Device %s unblocked successfully", c.Args()[1]))
		}
	case "/block":
		if err := t.router.FilterDeviceByMac(c.Args()[1]); err != nil {
			return c.Send("Failed to block the device")
		} else {
			return c.Send(fmt.Sprintf("Device %s blocked successfully", c.Args()[1]))
		}
	}

	return c.Send("Invalid command")
}

func NewTelegramBot(r routers.Router) *TelegramBot {
	return &TelegramBot{
		router:    r,
		authGroup: make(map[int64]bool, 0),
		nameCache: make(map[string]string, 0),
	}
}
