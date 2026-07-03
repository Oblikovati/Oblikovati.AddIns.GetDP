// SPDX-License-Identifier: GPL-2.0-only

package getdp

import (
	"oblikovati.org/api/types"
	"oblikovati.org/api/wire"
)

// RibbonTab is the dedicated ribbon tab every GetDP command lands on (design spec
// §4.1). Registered on the Part ribbon: the wire contract binds one command id to one
// ribbon, so the Assembly-ribbon registration joins when assembly studies ship (M10
// demos) — likely via an api extension carrying multiple ribbons per command.
const RibbonTab = "GetDP"

// Panel names in workflow order (spec §4.1). Results/Optimization/Help panels join
// with their milestones — unshipped commands are absent, not disabled.
var getdpPanels = []string{"Study", "Setup", "Mesh", "Solve", "Windows", "Demos"}

// ribbonSpot places one command on a panel of the GetDP tab, with its inline glyph
// (an embedded icons/<icon>.svg — see iconSVG), button style, control kind and popup
// items. A spot with panel == "" is popup-only: reachable through a flyout, never
// placed directly.
type ribbonSpot struct {
	panel string
	icon  string
	style types.ButtonStyle
	kind  types.ControlKind
	items []string
}

// getdpRibbonSpots places every GetDP command. Kept exhaustive so a command can never
// land on an unnamed panel or without a glyph — guarded by the ribbon layout tests.
var getdpRibbonSpots = map[string]ribbonSpot{
	// Study — one flyout head plus per-physics variants (popup-only).
	NewStudyCommandID: {panel: "Study", icon: "newstudy", style: types.LargeIconButton,
		kind: types.PopupControl, items: []string{
			NewElectrokineticsCommandID, NewThermalCommandID, NewThermalTransientCommandID}},
	NewElectrokineticsCommandID:  {icon: "elekin", style: types.SmallIconButton},
	NewThermalCommandID:          {icon: "thermal", style: types.SmallIconButton},
	NewThermalTransientCommandID: {icon: "thermaltransient", style: types.SmallIconButton},
	StudySettingsCommandID:       {panel: "Study", icon: "studysettings", style: types.SmallIconButton},
	DuplicateStudyCommandID:      {panel: "Study", icon: "duplicate", style: types.SmallIconButton},
	SetActiveStudyCommandID:      {panel: "Study", icon: "setactive", style: types.SmallIconButton},

	// Setup — region/material scoping and the BC flyout (variants popup-only; the
	// engine validates kind×physics on click, femmodel is the gate).
	EditRegionsCommandID: {panel: "Setup", icon: "regions", style: types.LargeIconButton},
	AddBCCommandID: {panel: "Setup", icon: "boundarycondition", style: types.LargeIconButton,
		kind: types.PopupControl, items: []string{
			AddVoltageCommandID, AddCurrentCommandID,
			AddTemperatureCommandID, AddHeatFluxCommandID, AddConvectionCommandID}},
	AddVoltageCommandID:     {icon: "elekin", style: types.SmallIconButton},
	AddCurrentCommandID:     {icon: "elekin", style: types.SmallIconButton},
	AddTemperatureCommandID: {icon: "thermal", style: types.SmallIconButton},
	AddHeatFluxCommandID:    {icon: "thermal", style: types.SmallIconButton},
	AddConvectionCommandID:  {icon: "thermal", style: types.SmallIconButton},
	EditMaterialsCommandID:  {panel: "Setup", icon: "materials", style: types.SmallIconButton},
	AirRegionCommandID:      {panel: "Setup", icon: "airregion", style: types.SmallIconButton},

	// Mesh.
	GenerateMeshCommandID: {panel: "Mesh", icon: "meshgen", style: types.LargeIconButton},
	MeshSettingsCommandID: {panel: "Mesh", icon: "meshsettings", style: types.SmallIconButton},

	// Solve.
	RunStudyCommandID:       {panel: "Solve", icon: "solve", style: types.LargeIconButton},
	SolverSettingsCommandID: {panel: "Solve", icon: "solversettings", style: types.SmallIconButton},
	StopSolveCommandID:      {panel: "Solve", icon: "stop", style: types.SmallIconButton},

	// Windows.
	ShowTreeCommandID:    {panel: "Windows", icon: "tree", style: types.SmallIconButton},
	ShowMonitorCommandID: {panel: "Windows", icon: "monitor", style: types.SmallIconButton},

	// Demos — bundled parametric tutorials (the M10 Help flyout regroups these).
	DemoBusbarCommandID:   {panel: "Demos", icon: "elekin", style: types.LargeIconButton},
	DemoHeatSinkCommandID: {panel: "Demos", icon: "thermal", style: types.LargeIconButton},
}

// commandArgs builds the host command-registration args, placing the command on its
// GetDP-tab panel with its bundled glyph. Popup-only variants carry no tab/panel.
func commandArgs(id, name, tip string) wire.CreateCommandArgs {
	spot := getdpRibbonSpots[id]
	args := wire.CreateCommandArgs{
		ID: id, DisplayName: name, Tooltip: tip,
		IconSVG: iconSVG(spot.icon), ButtonStyle: spot.style,
		Kind: spot.kind, Items: spot.items,
	}
	if spot.panel != "" {
		args.Ribbon, args.Tab, args.Category = types.PartRibbon, RibbonTab, spot.panel
	}
	return args
}
