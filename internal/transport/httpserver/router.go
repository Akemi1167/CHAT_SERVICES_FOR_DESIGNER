package httpserver

import (
	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/botapi"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/clientapi"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/handler"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/ws"
)

// registerRoutes: toan bo route cua service nam o day => mo file la thay het API.
func registerRoutes(app *fiber.App, cfg *config.Config, d Deps) {
	convH := handler.NewConversationHandler(d.Conversations)
	msgH := handler.NewMessageHandler(d.Messages)
	botH := handler.NewBotHandler(d.Bots, d.Messages)
	taskH := handler.NewTaskHandler(d.Tasks)
	clientH := clientapi.NewHandler(d.Store, d.Conversations, d.Messages, d.Cards, d.Notifications, d.CMSSync)
	cmsBotH := botapi.NewHandler(d.Store, d.CMSSync, d.Messages, d.Cards, d.Notifications)
	wsH := ws.NewHandler(cfg.HTTP, d.Hub, d.Messages)

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

	// --- WebSocket: /ws?token=<JWT> ---
	app.Use("/ws", middleware.RequestID(), middleware.AuthWS(d.Auth), wsH.Upgrade)
	app.Get("/ws", wsH.Handle())

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

	msg := api.Group("/messages")
	msg.Get("/search", msgH.Search)
	msg.Get("/:message_id/replies", msgH.Replies)
	msg.Get("/:message_id/readers", msgH.Readers)
	msg.Delete("/:message_id", msgH.Delete)

	// --- task noi bo: CHI bat khi TASK_ENGINE_ENABLED=true ---
	// Khi tich hop CMS, don/task do CMS so huu: Chat chi luu card va chuyen event
	// (xem docs/10-cms-integration.md). Code task van giu lai de dung doc lap.
	if cfg.App.TaskEngine {
		registerTaskRoutes(api, taskH)
	}

	bots := api.Group("/bots")
	bots.Get("/", botH.ListBots)
	bots.Get("/:slug/templates", botH.ListTemplates)
	bots.Post("/:slug/submissions", botH.Submit)
	bots.Get("/:slug/submissions", botH.ListSubmissions)
}

// registerTaskRoutes: module task noi bo (tat mac dinh khi dung CMS).
func registerTaskRoutes(api fiber.Router, taskH *handler.TaskHandler) {
	tasks := api.Group("/tasks")
	tasks.Post("/", taskH.Create)
	tasks.Get("/", taskH.List)
	tasks.Get("/code/:code", taskH.GetByCode)
	tasks.Get("/:id", taskH.Get)
	tasks.Patch("/:id", taskH.Update)
	tasks.Get("/:id/events", taskH.Events)
	tasks.Post("/:id/accept", taskH.Accept)
	tasks.Post("/:id/pause", taskH.Pause)
	tasks.Post("/:id/resume", taskH.Resume)
	tasks.Post("/:id/hold", taskH.Hold)
	tasks.Post("/:id/complete", taskH.Complete)
	tasks.Post("/:id/assignees", taskH.AddAssignees)
	tasks.Post("/:id/reassign", taskH.Reassign)
	tasks.Post("/:id/cancel", taskH.Cancel)
	tasks.Get("/:id/revisions", taskH.ListRevisions)
	tasks.Post("/:id/revisions", taskH.CreateRevision)

	api.Post("/revisions/:revision_id/complete", taskH.CompleteRevision)

	teams := api.Group("/teams")
	teams.Get("/", taskH.ListTeams)
	teams.Post("/", taskH.CreateTeam)
	teams.Get("/:id/members", taskH.ListTeamMembers)
	teams.Post("/:id/members", taskH.AddTeamMembers)
	teams.Delete("/:id/members/:user_id", taskH.RemoveTeamMember)
	teams.Get("/:id/suggest", taskH.Suggest)
}
