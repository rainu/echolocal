//go:build !board_biscuit

package settings

import (
	"sync"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/media/cast/protocols/webreceiver"
	"github.com/ygelfand/libcountertop/pkg/say"

	"github.com/ygelfand/echolocal/internal/feature/localvideo"
	"github.com/ygelfand/echolocal/internal/feature/shell"
)

func youtubeText(key string) string {
	all := map[string]map[string]string{
		"en": {
			"checking":     "Checking connection…",
			"disconnected": "Not connected",
			"linking":      "Waiting for Google confirmation",
			"connected":    "Connected",
			"expired":      "Code expired — try again",
			"error":        "Connection failed",
			"status":       "Connection",
			"premium":      "Premium active",
			"code":         "Activation code",
			"hint":         "Confirm on your phone or computer. Passwords stay with Google.",
			"connect":      "Sign in / try again",
			"refresh":      "Check connection",
			"cancel":       "Cancel sign-in",
			"disconnect":   "Sign out on this device",
			"confirm":      "Remove this device’s local YouTube session and consent data? Playback stops. Your Google account and subscription are unchanged; remote authorization is not revoked.",
			"remove":       "Sign out",
			"back":         "Cancel",
		},
		"de": {
			"checking":     "Verbindung wird geprüft…",
			"disconnected": "Nicht verbunden",
			"linking":      "Warte auf Google-Bestätigung",
			"connected":    "Verbunden",
			"expired":      "Code abgelaufen — erneut versuchen",
			"error":        "Verbindung fehlgeschlagen",
			"status":       "Verbindung",
			"premium":      "Premium aktiv",
			"code":         "Aktivierungscode",
			"hint":         "Am Handy oder Computer bestätigen. Passwörter bleiben bei Google.",
			"connect":      "Anmelden / erneut versuchen",
			"refresh":      "Verbindung prüfen",
			"cancel":       "Anmeldung abbrechen",
			"disconnect":   "Auf diesem Gerät abmelden",
			"confirm":      "Lokale YouTube-Sitzung und Einwilligungsdaten auf diesem Gerät entfernen? Wiedergabe stoppt. Google-Konto und Abo bleiben unverändert; die Freigabe bei Google wird nicht widerrufen.",
			"remove":       "Abmelden",
			"back":         "Abbrechen",
		},
		"it": {
			"checking":     "Verifica connessione…",
			"disconnected": "Non connesso",
			"linking":      "In attesa di conferma Google",
			"connected":    "Connesso",
			"expired":      "Codice scaduto — riprova",
			"error":        "Connessione non riuscita",
			"status":       "Connessione",
			"premium":      "Premium attivo",
			"code":         "Codice di attivazione",
			"hint":         "Conferma dal telefono o computer. Le password restano con Google.",
			"connect":      "Accedi / riprova",
			"refresh":      "Verifica connessione",
			"cancel":       "Annulla accesso",
			"disconnect":   "Esci da questo dispositivo",
			"confirm":      "Rimuovere sessione YouTube e consenso locali? La riproduzione si ferma. Account e abbonamento Google invariati; autorizzazione remota non revocata.",
			"remove":       "Esci",
			"back":         "Annulla",
		},
		"vi": {
			"checking":     "Đang kiểm tra…",
			"disconnected": "Chưa kết nối",
			"linking":      "Đang chờ Google xác nhận",
			"connected":    "Đã kết nối",
			"expired":      "Mã hết hạn — thử lại",
			"error":        "Kết nối thất bại",
			"status":       "Kết nối",
			"premium":      "Premium đang hoạt động",
			"code":         "Mã kích hoạt",
			"hint":         "Xác nhận trên điện thoại hoặc máy tính. Mật khẩu chỉ nhập tại Google.",
			"connect":      "Đăng nhập / thử lại",
			"refresh":      "Kiểm tra kết nối",
			"cancel":       "Hủy đăng nhập",
			"disconnect":   "Đăng xuất trên thiết bị này",
			"confirm":      "Xóa phiên YouTube và dữ liệu đồng ý cục bộ? Phát sẽ dừng. Tài khoản và gói Google không thay đổi; quyền từ xa không bị thu hồi.",
			"remove":       "Đăng xuất",
			"back":         "Hủy",
		},
	}
	lang := say.Chosen()
	if all[lang] == nil {
		lang = "en"
	}
	return all[lang][key]
}

var youtubeListen sync.Once

func youtubePage() *sharedshell.Page {
	a := localvideo.Account()
	youtubeListen.Do(
		func() { a.Changed.Listen(func(webreceiver.YouTubeConnection) { shell.Get().Redraw() }) },
	)
	command := func(action string) { _ = a.Command(action); shell.Get().Redraw() }
	return &sharedshell.Page{
		Title: "YouTube",
		Build: func() ([]menu.Row, []func(int)) {
			s := a.Snapshot()
			rows := []menu.Row{
				{Label: youtubeText("status"), Value: youtubeText(s.State), Kind: menu.Plain},
			}
			if s.Premium {
				rows = append(rows, menu.Row{Label: youtubeText("premium"), Kind: menu.Plain})
			}
			if s.Code != "" {
				rows = append(
					rows,
					menu.Row{Label: youtubeText("code"), Value: s.Code, Kind: menu.Plain},
					menu.Row{Label: "yt.be/activate", Hint: youtubeText("hint"), Kind: menu.Plain},
				)
			}
			if s.Error != "" {
				rows = append(
					rows,
					menu.Row{
						Label: youtubeText("error"),
						Hint:  youtubeText("connect"),
						Kind:  menu.Plain,
					},
				)
			}
			taps := make([]func(int), len(rows))
			if s.State != "connected" && s.State != "checking" {
				rows = append(
					rows,
					menu.Row{
						Label: youtubeText("connect"),
						Hint:  youtubeText("hint"),
						Kind:  menu.Plain,
					},
				)
				taps = append(taps, func(int) { command("connect") })
			}
			if s.State == "linking" {
				rows = append(rows, menu.Row{Label: youtubeText("cancel"), Kind: menu.Plain})
				taps = append(taps, func(int) { command("cancel") })
			}
			return rows, taps
		},
		Actions: []sharedshell.Action{
			{
				Label:     youtubeText("refresh"),
				MinHeight: 64,
				Role:      sharedshell.Plain,
				Do:        func() { command("refresh") },
			},
			{Label: youtubeText("disconnect"), MinHeight: 64, Role: sharedshell.Danger, Do: func() {
				shell.Get().
					Push(&sharedshell.Page{Title: youtubeText("disconnect"), Build: func() ([]menu.Row, []func(int)) {
						return []menu.Row{
							{Label: youtubeText("confirm"), Kind: menu.Plain},
						}, []func(int){
							nil,
						}
					},
						Actions: []sharedshell.Action{
							{
								Label:     youtubeText("back"),
								MinHeight: 64,
								Do:        func() { shell.Get().Pop() },
							},
							{
								Label:     youtubeText("remove"),
								MinHeight: 64,
								Role:      sharedshell.Danger,
								Do:        func() { command("disconnect"); shell.Get().Pop() },
							},
						},
					})
			}},
		},
	}
}
