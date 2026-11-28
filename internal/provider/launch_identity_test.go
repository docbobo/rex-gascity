package provider

import (
	"reflect"
	"testing"
)

// TestLaunchIdentity pins the names one start env produces. The shapes used to
// live in separate files: the town and rig tab rules in sessionname_test.go,
// the legacy labels in tablabel_test.go, and the pane labels and aliases in
// pane_label_test.go and binding_test.go. The launch-identity module owns the
// env reads, so one table proves them together.
func TestLaunchIdentity(t *testing.T) {
	tests := []struct {
		name      string
		sess      SessionName
		env       map[string]string
		wantTab   string
		wantPane  string
		wantAlias []SessionName
		wantTabs  []string
		wantLPane string
		wantClaim string
	}{
		{
			name:      "town name without GC_RIG",
			sess:      "mayor",
			env:       map[string]string{"GC_ALIAS": "mayor", "GC_AGENT": "mayor", "GC_SESSION_NAME": "mayor", "GC_SESSION_ID": "mc-123"},
			wantTab:   "[city] my-city",
			wantPane:  "[mayor] (mayor)",
			wantTabs:  []string{"__town"},
			wantLPane: "mayor",
			wantClaim: "mc-123",
		},
		{
			name:      "town-style wisp name with GC_RIG",
			sess:      "gc__worker-mc-wisp-aaaa",
			env:       map[string]string{"GC_RIG": "rex-gascity", "GC_ALIAS": "gc.worker", "GC_AGENT": "mc-wisp-aaaa", "GC_SESSION_NAME": "gc__worker-mc-wisp-aaaa", "GC_SESSION_ID": "mc-456"},
			wantTab:   "[rig] rex-gascity",
			wantPane:  "[gc.worker] (gc__worker-mc-wisp-aaaa) #mc-456",
			wantAlias: []SessionName{"gc__worker", "rex-gascity--gc__worker-mc-wisp-aaaa"},
			wantTabs:  []string{"__town", "rex-gascity"},
			wantLPane: "gc.worker (gc__worker-mc-wisp-aaaa)",
			wantClaim: "mc-456",
		},
		{
			name:      "rig name without GC_RIG",
			sess:      "Indigo--dog-1",
			env:       map[string]string{},
			wantTab:   "[rig] Indigo",
			wantPane:  "dog-1",
			wantTabs:  []string{"Indigo"},
			wantLPane: "dog-1",
		},
		{
			name:      "short pool name",
			sess:      "gc__short-mc-wisp-cccc",
			env:       map[string]string{"GC_RIG": "rex-gascity", "GC_TEMPLATE": "rex-gascity/gc.short", "GC_AGENT": "mc-wisp-cccc", "GC_SESSION_NAME": "gc__short-mc-wisp-cccc", "GC_SESSION_ID": "mc-ccc"},
			wantTab:   "[rig] rex-gascity",
			wantPane:  "[gc.short] (#mc-wisp-cccc) #mc-ccc",
			wantAlias: []SessionName{"rex-gascity--gc__short", "rex-gascity--gc__short-mc-wisp-cccc"},
			wantTabs:  []string{"__town", "rex-gascity"},
			wantLPane: "mc-wisp-cccc (gc__short-mc-wisp-cccc)",
			wantClaim: "mc-ccc",
		},
		{
			name:      "pre-GC_TEMPLATE alias",
			sess:      "rig--agent",
			env:       map[string]string{"GC_RIG": "rig", "GC_ALIAS": "agent-2", "GC_SESSION_NAME": "rig--agent"},
			wantTab:   "[rig] rig",
			wantPane:  "[agent-2] (rig--agent)",
			wantAlias: []SessionName{"agent-2"},
			wantTabs:  []string{"rig"},
			wantLPane: "agent-2 (rig--agent)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newLaunchIdentity(tt.sess, tt.env, "my-city")
			if got.TabLabel() != tt.wantTab {
				t.Errorf("TabLabel() = %q, want %q", got.TabLabel(), tt.wantTab)
			}
			if got.PaneLabel() != tt.wantPane {
				t.Errorf("PaneLabel() = %q, want %q", got.PaneLabel(), tt.wantPane)
			}
			if !reflect.DeepEqual(got.Aliases(), tt.wantAlias) {
				t.Errorf("Aliases() = %v, want %v", got.Aliases(), tt.wantAlias)
			}
			if !reflect.DeepEqual(got.LegacyTabLabels(), tt.wantTabs) {
				t.Errorf("LegacyTabLabels() = %v, want %v", got.LegacyTabLabels(), tt.wantTabs)
			}
			if got.LegacyPaneLabel() != tt.wantLPane {
				t.Errorf("LegacyPaneLabel() = %q, want %q", got.LegacyPaneLabel(), tt.wantLPane)
			}
			if got.ClaimSessionID() != tt.wantClaim {
				t.Errorf("ClaimSessionID() = %q, want %q", got.ClaimSessionID(), tt.wantClaim)
			}
		})
	}
}
