package model

import "encoding/json"

const PODStudioAccess = "pod:studio:access"
const PODGraphVersion = 1

type PODGraph struct {
	SchemaVersion int       `json:"schemaVersion"`
	Nodes         []PODNode `json:"nodes"`
	Edges         []PODEdge `json:"edges"`
}

type PODNode struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Version  int             `json:"version"`
	Position PODPosition     `json:"position"`
	Config   json.RawMessage `json:"config"`
}

type PODPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type PODEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	SourcePort string `json:"sourcePort"`
	Target     string `json:"target"`
	TargetPort string `json:"targetPort"`
}
type PODPort struct {
	Key       string `json:"key"`
	ValueType string `json:"valueType"`
	Required  bool   `json:"required"`
}
type PODNodeDefinition struct {
	Type        string    `json:"type"`
	Version     int       `json:"version"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	Inputs      []PODPort `json:"inputs"`
	Outputs     []PODPort `json:"outputs"`
	Executable  bool      `json:"executable"`
}
type PODWorkspace struct {
	WorkspaceID int64  `gorm:"column:workspace_id" json:"workspaceId,string"`
	Name        string `gorm:"column:name" json:"name"`
}
type PODBootstrap struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Workspace     PODWorkspace        `json:"workspace"`
	UserID        string              `json:"userId"`
	UserName      string              `json:"userName"`
	NickName      string              `json:"nickName"`
	NodeTypes     []PODNodeDefinition `json:"nodeTypes"`
	Capabilities  PODCapabilities     `json:"capabilities"`
	Limits        PODGraphLimits      `json:"limits"`
	AssetLimits   PODAssetLimits      `json:"assetLimits"`
	Execution     PODExecutionLimits  `json:"execution"`
}

type PODGraphLimits struct {
	MaxNodes      int `json:"maxNodes"`
	MaxEdges      int `json:"maxEdges"`
	MaxGraphBytes int `json:"maxGraphBytes"`
}
type PODCapabilities struct {
	ProjectEditing bool `json:"projectEditing"`
	Execution      bool `json:"execution"`
	Assets         bool `json:"assets"`
}

// PODRunState contracts are defined now; execution is implemented in step 4.
type PODRunState string

const (
	PODRunQueued                 PODRunState = "queued"
	PODRunRunning                PODRunState = "running"
	PODRunSucceeded              PODRunState = "succeeded"
	PODRunFailed                 PODRunState = "failed"
	PODRunCancelRequested        PODRunState = "cancelRequested"
	PODRunCancelled              PODRunState = "cancelled"
	PODRunAwaitingReconciliation PODRunState = "awaitingReconciliation"
)
