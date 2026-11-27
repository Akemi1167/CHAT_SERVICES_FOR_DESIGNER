package httpserver

import (
	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/botapi"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/clientapi"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/clientview"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/handler"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/ws"
)

// registerRoutes: toan bo route cua service nam o day => mo file la thay het API.
func registerRoutes(app *fiber.App, cfg *config.Config, d Deps) {
	convH := handler.NewConversationHandler(d.Conversations, d.Store)
	msgH := handler.NewMessageHandler(d.Messages, d.Store)
	botH := handler.NewBotHandler(d.Bots, d.Messages)
	clientH := clientapi.NewHandler(d.Store, d.Conversations, d.Messages, d.Cards, d.Notifications, d.CMSSync)
	cmsBotH := botapi.NewHandler(d.Store, d.CMSSync, d.Messages, d.Cards, d.Notifications, d.Presence)
	wsH := ws.NewHandler(cfg.HTTP, d.Hub, d.Messages, d.Store)

	// Hub khong biet gi ve hop dong CMS; tiem bo doi payload tu tang transport.
	d.Hub.SetClientRenderer(clientview.Payload)

	// --- public: health check cho k8s/docker ---
	app.Get("/healthz", func(c *fiber.Ctx) error {
		return httpresp.OK(c, fiber.Map{"status": "ok"})
	})
	app.Get("/readyz", func(c *fiber.Ctx) error {
		if d.Ready != nil {
			if err := d.Ready(c.UserContext()); err != nil {
				return httpresp.Fail(c, err)
			}
		}
		return httpresp.OK(c, fiber.Map{"status": "ready"})
	})

	if cfg.HTTP.WebDemoDir != "" {
		app.Static("/demo", cfg.HTTP.WebDemoDir, fiber.Static{Browse: true})
	}

	// --- Tai lieu: tra docs/FLOW.md cho trang doc ben ngoai (khoa rieng tren header) ---
	// Chi mo khi co DOCS_API_KEY; khong co thi route khong ton tai (404).
	if cfg.HTTP.DocsKey != "" {
		app.Get("/docs/flow", middleware.RequestID(), middleware.AccessLog(),
			docsKeyAuth(cfg.HTTP.DocsKey), flowDoc)
	}

	// --- Log trong DB (app_logs) de debug: chi mo khi co DEBUG_API_KEY ---
	if cfg.HTTP.DebugKey != "" && d.DBLogs != nil {
		app.Get("/debug/logs", middleware.RequestID(), debugKeyAuth(cfg.HTTP.DebugKey), debugLogs(d.DBLogs))
	}

	// --- WebSocket ---
	// Hai duong vao, CUNG mot handler va cung mot hub, khac nhau hinh dang frame:
	//
	//   /ws?token=<JWT>                      snake_case, payload nguyen domain
	//                                        (cong cu noi bo: make ws, Postman)
	//   /api/client/v1/chat/ws?token=<JWT>   camelCase theo hop dong CMS, dung
	//                                        chung access token voi REST cua FE
	//
	// Browser khong set duoc header khi mo WebSocket nen token di qua ?token=.
	app.Use("/ws", middleware.RequestID(), middleware.AuthWS(d.Auth), wsH.Upgrade)
	app.Get("/ws", wsH.Handle(realtime.DialectInternal))

	app.Use("/api/client/v1/chat/ws",
		middleware.RequestID(), middleware.AuthWS(d.Auth), wsH.Upgrade)
	app.Get("/api/client/v1/chat/ws", wsH.Handle(realtime.DialectClient))

	// --- Bot API: CMS goi sang Chat (service key, khong dung JWT) ---
	if cfg.CMS.Enabled {
		bot := app.Group("/bot/v1", middleware.RequestID(), middleware.AccessLog(),
			botapi.ServiceKeyAuth(cfg.CMS.ServiceKey))
		cmsBotH.Register(bot)
	}

	// --- API cho web chat theo hop dong CMS: envelope {statusCode, code, message, element} ---
	clientH.Register(app.Group("/api/client/v1/chat",
		middleware.RequestID(), middleware.AccessLog(), middleware.Auth(d.Auth)))

	// --- REST (ban goc cua Chat, dung cho Postman/CLI/service noi bo) ---
	api := app.Group("/api/v1", middleware.RequestID(), middleware.AccessLog(), middleware.Auth(d.Auth))

	// Debug realtime: ai dang mo WebSocket tren instance nay.
	api.Get("/ws/online", handler.NewWSStatusHandler(d.Hub, d.Store).Online)

	api.Get("/me", func(c *fiber.Ctx) error {
		id, err := middleware.Identity(c)
		if err != nil {
			return httpresp.Fail(c, err)
		}
		return httpresp.OK(c, id)
	})

	conv := api.Group("/conversations")
	conv.Post("/", convH.Create)
	conv.Get("/", convH.List)
	conv.Get("/:id", convH.Get)
	conv.Post("/:id/members", convH.AddMembers)
	conv.Delete("/:id/members/:user_id", convH.RemoveMember)
	conv.Get("/:id/messages", msgH.List)
	conv.Post("/:id/messages", msgH.Send)
	conv.Post("/:id/read", msgH.MarkRead)
	conv.Get("/:id/pinned", msgH.ListPinned)

	msg := api.Group("/messages")
	msg.Get("/search", msgH.Search)
	msg.Get("/:message_id/replies", msgH.Replies)
	msg.Post("/:message_id/pin", msgH.Pin)
	msg.Get("/:message_id/attachments/:attachment_id", msgH.Attachment)
	msg.Get("/:message_id/readers", msgH.Readers)
	msg.Delete("/:message_id", msgH.Delete)

	bots := api.Group("/bots")
	bots.Get("/", botH.ListBots)
	bots.Get("/:slug/templates", botH.ListTemplates)
	bots.Post("/:slug/submissions", botH.Submit)
	bots.Get("/:slug/submissions", botH.ListSubmissions)
}
