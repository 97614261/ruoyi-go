package model

import "ruoyi-go/pkg/types"

const (
	PODProductText                 = "product-text"
	PODProductReference            = "product-reference"
	PODProductMain                 = "product-main"
	PODProductScene                = "product-scene"
	PODProductAngle                = "product-angle"
	PODProductSide                 = "product-side"
	PODProductDetail               = "product-detail"
	PODProductImages               = "product"
	PODProductPrint                = "print"
	PODProductMockup               = "mockup"
	PODProductDefaultOutputs uint8 = 3
)

type PODProductOutput struct {
	ID       string
	Label    string
	Filename string
}

var podProductOutputs = []PODProductOutput{
	{PODProductMain, "白底主图", "main-white"},
	{PODProductScene, "场景图", "scene"},
	{PODProductAngle, "45° 展示图", "angle-45"},
	{PODProductSide, "侧面图", "side"},
	{PODProductDetail, "细节特写", "detail"},
}

func PODProductOutputMask(id string) uint8 {
	for i, output := range podProductOutputs {
		if output.ID == id {
			return 1 << i
		}
	}
	return 0
}

func PODProductOutputs(mask uint8) []PODProductOutput {
	outputs := make([]PODProductOutput, 0, len(podProductOutputs))
	for i, item := range podProductOutputs {
		if mask&(1<<i) != 0 {
			outputs = append(outputs, item)
		}
	}
	return outputs
}

type PODProductFields struct {
	ProductName      string `json:"productName"`
	SKU              string `json:"sku"`
	SellingPoints    string `json:"sellingPoints"`
	ProtectedDetails string `json:"protectedDetails"`
}

type PODProductPack struct {
	PackID      int64  `gorm:"primaryKey" json:"packId,string"`
	WorkspaceID int64  `json:"-"`
	Fingerprint string `json:"-"`
	PODProductFields
	ReferenceAssetID int64      `json:"referenceAssetId,string"`
	CreateTime       types.Time `json:"createTime"`
}

func (PODProductPack) TableName() string { return "pod_product_pack" }

type PODProductCreate struct {
	PODProductFields
	ReferenceAssetID string    `json:"referenceAssetId"`
	ModelID          string    `json:"modelId"`
	Ratio            string    `json:"ratio"`
	RequestID        string    `json:"requestId"`
	WorkKind         string    `json:"workKind,omitempty"`
	Outputs          *[]string `json:"outputs,omitempty"`
}

type PODProductJob struct {
	ProjectID   int64  `gorm:"primaryKey;autoIncrement:false" json:"projectId,string"`
	WorkspaceID int64  `json:"-"`
	PackID      int64  `json:"packId,string"`
	RequestID   string `json:"-"`
	RequestHash string `json:"-"`
	WorkKind    string `json:"-"`
	OutputMask  uint8  `json:"-"`
}

func (PODProductJob) TableName() string { return "pod_product_job" }

type PODProductRecent struct {
	ProjectID int64  `json:"projectId,string"`
	Name      string `json:"name"`
	WorkKind  string `json:"workKind"`
}

type PODProductRun struct {
	RunID           int64        `json:"runId,string"`
	ProjectRevision int64        `json:"projectRevision,string"`
	RunState        PODRunState  `json:"runState"`
	State           string       `json:"state"`
	ErrorCode       string       `json:"errorCode,omitempty"`
	ErrorMessage    string       `json:"errorMessage,omitempty"`
	TestOnly        bool         `json:"testOnly,omitempty"`
	Output          PODRunOutput `gorm:"-" json:"output"`
}

type PODProductSlot struct {
	NodeID string               `json:"nodeId"`
	Latest *PODProductRun       `json:"latest,omitempty"`
	Result *PODProductRun       `json:"result,omitempty"`
	Final  *PODProductFinalView `json:"final,omitempty"`
}

type PODProductFinal struct {
	ProjectID   int64  `gorm:"primaryKey;autoIncrement:false"`
	NodeID      string `gorm:"primaryKey"`
	WorkspaceID int64
	RunID       int64
	AssetID     int64
}

func (PODProductFinal) TableName() string { return "pod_product_final" }

type PODProductFinalView struct {
	RunID           int64 `json:"runId,string"`
	AssetID         int64 `json:"assetId,string"`
	ProjectRevision int64 `json:"projectRevision,string"`
	TestOnly        bool  `json:"testOnly,omitempty"`
}

type PODProductFinalChange struct {
	RunID           string `json:"runId"`
	AssetID         string `json:"assetId"`
	PreviousRunID   string `json:"previousRunId"`
	PreviousAssetID string `json:"previousAssetId"`
}

type PODProductView struct {
	Project  *PODProjectView  `json:"project"`
	Pack     PODProductPack   `json:"pack"`
	Slots    []PODProductSlot `json:"slots"`
	QuickRun bool             `json:"quickRun"`
	WorkKind string           `json:"workKind"`
}
