package react

import (
	"testing"

	"github.com/wangliang139/NovaForge/server/pkg/chat/internal/tools"
)

func TestShouldInvokeBuiltinTool(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{in: "请给我当前时间", want: true},
		{in: "现在几点了", want: true},
		{in: "做个json 回显", want: true},
		{in: "解释一下 k 线", want: false},
	}
	for _, c := range cases {
		if got := shouldInvokeBuiltinTool(c.in); got != c.want {
			t.Fatalf("input=%q, got=%v, want=%v", c.in, got, c.want)
		}
	}
}

func TestSelectBuiltinTool(t *testing.T) {
	name, _ := selectBuiltinTool("请返回当前时间")
	if name != tools.ToolNowISO8601 {
		t.Fatalf("unexpected tool name: %s", name)
	}
	name, args := selectBuiltinTool("做一个json 回显")
	if name != tools.ToolEchoJSON {
		t.Fatalf("unexpected tool name: %s", name)
	}
	if args["payload"] == nil {
		t.Fatalf("expected payload in args")
	}
}
