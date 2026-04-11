package chat

import (
	"context"

	"github.com/wangliang139/NovaForge/server/pkg/chat/domain"
	chatcfg "github.com/wangliang139/NovaForge/server/pkg/chat/internal/config"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/coordinator"
	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/manager"
	llmentity "github.com/wangliang139/NovaForge/server/pkg/entity/llm"
	"github.com/wangliang139/NovaForge/server/pkg/internal/zai"
	"github.com/wangliang139/NovaForge/server/pkg/repos"
)

type Service struct {
	mgr   *manager.Manager
	coord *coordinator.Coordinator
}

func NewService(db *repos.Entity, llm *llmentity.Entity) *Service {
	cfg := chatcfg.Load()
	eng := zai.NewEngine()
	mgr := manager.New(db, eng, llm)
	return &Service{
		mgr:   mgr,
		coord: coordinator.New(mgr, cfg),
	}
}

func (s *Service) ListModels(ctx context.Context) ([]LlmModelItem, error) {
	return s.mgr.ListModels(ctx)
}

func (s *Service) ListSessions(ctx context.Context, userID int64, limit, offset int32) ([]SessionDTO, error) {
	return s.mgr.ListSessions(ctx, userID, limit, offset)
}

func (s *Service) GetSession(ctx context.Context, userID, sessionID int64) (*SessionDetailDTO, error) {
	return s.mgr.GetSession(ctx, userID, sessionID)
}

func (s *Service) DeleteSession(ctx context.Context, userID, sessionID int64) error {
	return s.mgr.DeleteSession(ctx, userID, sessionID)
}

func (s *Service) ChatStream(ctx context.Context, req domain.ChatRequest) (<-chan domain.DeltaEvent, error) {
	return s.coord.ChatStream(ctx, req)
}

func (s *Service) GenerateSessionTitle(ctx context.Context, userID, sessionID int64) (string, error) {
	return s.mgr.GenerateSessionTitle(ctx, userID, sessionID)
}

func (s *Service) UpdateSessionTitle(ctx context.Context, userID, sessionID int64, title string) (string, error) {
	return s.mgr.UpdateSessionTitle(ctx, userID, sessionID, title)
}
