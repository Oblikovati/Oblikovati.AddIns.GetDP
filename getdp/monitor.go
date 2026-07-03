// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"fmt"

	"oblikovati.org/api/client"
	"oblikovati.org/api/types"
	"oblikovati.org/api/wire"
)

// MonitorWindowID is the Run Monitor dockable the solve pipeline narrates into.
const MonitorWindowID = "com.oblikovati.getdp.monitor"

// showMonitor (re)declares the Run Monitor with the current phase and an optional
// solver log tail. Best-effort: monitoring must never fail a run.
func (e *Engine) showMonitor(phase string, logTail []string) {
	controls := []wire.PanelControlSpec{
		client.PanelLabel("phase", "Status: "+phase),
		client.PanelButton("stop", "Stop", StopSolveCommandID),
	}
	if len(logTail) > 0 {
		controls = append(controls, client.PanelSeparator())
		for i, line := range logTail {
			controls = append(controls, client.PanelLabel(fmt.Sprintf("log%d", i), line))
		}
	}
	_, _ = e.api.DockableWindows().Set(wire.DockableWindowSpec{
		ID: MonitorWindowID, Title: "GetDP Run Monitor",
		Dock: types.DockBottom, Visible: true, Controls: controls,
	})
}
