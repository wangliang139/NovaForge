package capability_test

import (
	"testing"

	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/capability"
	// 触发 tools.init()，将内置能力注册到 capability 注册表。
	_ "github.com/wangliang139/NovaForge/server/pkg/chat/internal/tools"
)

func TestRegistryBasics(t *testing.T) {
	tools := capability.ListToolsFull()
	if len(tools) == 0 {
		t.Fatalf("expected builtin tools")
	}
	skills := capability.ListSkillsBrief()
	if len(skills) == 0 {
		t.Fatalf("expected builtin skills")
	}
	_, ok := capability.GetSkillDetail("获取策略开发手册")
	if !ok {
		t.Fatalf("expected to find skill detail")
	}
}
