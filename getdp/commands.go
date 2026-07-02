// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"oblikovati.org/api/types"
	"oblikovati.org/api/wire"
)

// RunStudyCommandID is the host command the add-in registers; firing it (a ribbon click
// or the MCP bridge's execute_command) runs the active GetDP study.
const RunStudyCommandID = "GetDP.RunStudy"

// RibbonTab is the dedicated ribbon tab every GetDP command lands on (design spec §4.1).
const RibbonTab = "GetDP"

// getdpCommands is the exhaustive command list; RegisterCommands places each on the
// GetDP tab. It grows panel by panel as the M3+ UI slices land (spec §4.1 is the
// target layout, guarded by an exhaustive layout test from #16 on).
var getdpCommands = []struct{ id, name, tip string }{
	{RunStudyCommandID, "Run Study", "Mesh, solve with GetDP, and visualize the active study's fields."},
}

// Setup performs the one-time host-facing initialization: register the ribbon commands.
// It MUST NOT run on the host's session goroutine (host calls there block until the frame
// loop drains the dispatcher, deadlocking the head) — the cgo shell runs it on its own
// goroutine. The study tree and dockables join in the M3 UI slice.
func (e *Engine) Setup() error {
	return e.RegisterCommands()
}

// RegisterCommands registers every GetDP command on the dedicated GetDP ribbon tab (also
// invokable over the MCP bridge's execute_command). Command actions fire command.started,
// which Notify dispatches.
func (e *Engine) RegisterCommands() error {
	for _, c := range getdpCommands {
		if _, err := e.api.Commands().Create(commandArgs(c.id, c.name, c.tip)); err != nil {
			return err
		}
	}
	return nil
}

// commandArgs builds the registration for one command on the GetDP tab. The full
// panel/icon/flyout placement map arrives with the M3 ribbon layout (#16).
func commandArgs(id, name, tip string) wire.CreateCommandArgs {
	return wire.CreateCommandArgs{
		ID: id, DisplayName: name, Tooltip: tip,
		Ribbon: types.PartRibbon, Tab: RibbonTab, Category: "Solve",
	}
}
