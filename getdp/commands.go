// SPDX-License-Identifier: GPL-2.0-only

package getdp

// Command ids of the M3 surface — grouped by ribbon panel (spec §4.1). Every id also
// fires over the MCP bridge's execute_command, which is how live tests drive the UI.
const (
	// Study panel.
	NewStudyCommandID            = "GetDP.NewStudy"
	NewElectrokineticsCommandID  = "GetDP.NewStudy.Electrokinetics"
	NewThermalCommandID          = "GetDP.NewStudy.Thermal"
	NewThermalTransientCommandID = "GetDP.NewStudy.ThermalTransient"
	StudySettingsCommandID       = "GetDP.StudySettings"
	DuplicateStudyCommandID      = "GetDP.DuplicateStudy"
	SetActiveStudyCommandID      = "GetDP.SetActiveStudy"

	// Setup panel.
	EditRegionsCommandID    = "GetDP.EditRegions"
	AddBCCommandID          = "GetDP.AddBoundaryCondition"
	AddVoltageCommandID     = "GetDP.AddBC.Voltage"
	AddCurrentCommandID     = "GetDP.AddBC.Current"
	AddTemperatureCommandID = "GetDP.AddBC.Temperature"
	AddHeatFluxCommandID    = "GetDP.AddBC.HeatFlux"
	AddConvectionCommandID  = "GetDP.AddBC.Convection"
	EditMaterialsCommandID  = "GetDP.EditMaterials"

	// Mesh panel.
	GenerateMeshCommandID = "GetDP.GenerateMesh"
	MeshSettingsCommandID = "GetDP.MeshSettings"

	// Solve panel.
	RunStudyCommandID       = "GetDP.RunStudy"
	SolverSettingsCommandID = "GetDP.SolverSettings"
	StopSolveCommandID      = "GetDP.StopSolve"

	// Windows panel.
	ShowTreeCommandID    = "GetDP.ShowTree"
	ShowMonitorCommandID = "GetDP.ShowMonitor"
)

// getdpCommands is the exhaustive command list; RegisterCommands places each per the
// ribbon layout (popup-only variants carry no panel).
var getdpCommands = []struct{ id, name, tip string }{
	{NewStudyCommandID, "New Study", "Create a simulation study on the active document."},
	{NewElectrokineticsCommandID, "Electrokinetics", "New steady current-conduction study (voltage/current electrodes, conductivity)."},
	{NewThermalCommandID, "Thermal", "New steady heat-conduction study (temperatures, fluxes, convection)."},
	{NewThermalTransientCommandID, "Thermal Transient", "New time-dependent heat-conduction study (theta time stepping)."},
	{StudySettingsCommandID, "Study Settings", "Edit the active study's physics and regime."},
	{DuplicateStudyCommandID, "Duplicate Study", "Copy the active study, settings and constraints included."},
	{SetActiveStudyCommandID, "Set Active", "Make the tree-selected study the active one."},

	{EditRegionsCommandID, "Regions", "Assign bodies and materials to the active study's regions."},
	{AddBCCommandID, "Boundary Condition", "Add a boundary condition from the current face selection."},
	{AddVoltageCommandID, "Voltage", "Fix the electric potential on the selected faces."},
	{AddCurrentCommandID, "Current", "Inject a total current through the selected faces."},
	{AddTemperatureCommandID, "Temperature", "Fix the temperature on the selected faces."},
	{AddHeatFluxCommandID, "Heat Flux", "Prescribe a total heat rate through the selected faces."},
	{AddConvectionCommandID, "Convection", "Apply a convection film (h, T∞) on the selected faces."},
	{EditMaterialsCommandID, "Materials", "Edit the active study's region material properties."},

	{GenerateMeshCommandID, "Generate Mesh", "Volume-mesh the study's bodies and report the element count."},
	{MeshSettingsCommandID, "Mesh Settings", "Edit the active study's global mesh controls."},

	{RunStudyCommandID, "Run Study", "Mesh, solve with GetDP, and visualize the active study's fields."},
	{SolverSettingsCommandID, "Solver Settings", "Edit the active study's solver and time-stepping controls."},
	{StopSolveCommandID, "Stop", "Cancel the running solve."},

	{ShowTreeCommandID, "Study Tree", "Open the GetDP study browser tree."},
	{ShowMonitorCommandID, "Run Monitor", "Open the GetDP run monitor window."},
}

// Setup performs the one-time host-facing initialization: register the ribbon
// commands and declare the study tree. It MUST NOT run on the host's session goroutine
// (host calls there block until the frame loop drains the dispatcher, deadlocking the
// head) — the cgo shell runs it on its own goroutine.
func (e *Engine) Setup() error {
	if err := e.RegisterCommands(); err != nil {
		return err
	}
	return e.ShowStudyTree()
}

// RegisterCommands registers every GetDP command per the ribbon layout (also invokable
// over the MCP bridge's execute_command). Command actions fire command.started, which
// Notify dispatches.
func (e *Engine) RegisterCommands() error {
	for _, c := range getdpCommands {
		if _, err := e.api.Commands().Create(commandArgs(c.id, c.name, c.tip)); err != nil {
			return err
		}
	}
	return nil
}
