package hotkey

import "strings"
import "testing"

func TestUpdateKHotkeysLeavesOtherToggles(t *testing.T) {
	in := "" +
		"[Data]\n" +
		"DataCount=1\n" +
		"\n" +
		"[Data_1]\n" +
		"Name=Other\n" +
		"Type=SIMPLE_ACTION_DATA\n" +
		"\n" +
		"[Data_1Actions0]\n" +
		"CommandURL=something toggle\n" +
		"Type=COMMAND_URL\n" +
		"\n" +
		"[Data_1 20 LinBoard]\n" +
		"Command=/old/linboard toggle\n" +
		"\n" +
		"[Main]\n" +
		"Disabled=false\n"

	out := UpdateKHotkeys(in, "/usr/bin/linboard toggle")
	if !strings.Contains(out, "CommandURL=something toggle") {
		t.Fatalf("other shortcut rewritten:\n%s", out)
	}
	if strings.Contains(out, "Data_1 20 LinBoard") {
		t.Fatalf("broken section kept:\n%s", out)
	}
	if !strings.Contains(out, "Name=LinBoard") || !strings.Contains(out, "CommandURL=/usr/bin/linboard toggle") {
		t.Fatalf("linboard entry missing:\n%s", out)
	}
	if !strings.Contains(out, "Key=Meta+V") || !strings.Contains(out, "DataCount=2") {
		t.Fatalf("binding or count missing:\n%s", out)
	}

	again := UpdateKHotkeys(out, "/opt/linboard toggle")
	if strings.Count(again, "Name=LinBoard") != 1 {
		t.Fatalf("duplicate groups:\n%s", again)
	}
	if !strings.Contains(again, "CommandURL=/opt/linboard toggle") {
		t.Fatalf("command not updated:\n%s", again)
	}
	if !strings.Contains(again, "CommandURL=something toggle") {
		t.Fatalf("other command lost:\n%s", again)
	}
}
