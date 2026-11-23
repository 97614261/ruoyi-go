// Package podsql embeds the reproducible, POD-only bootstrap schema.
package podsql

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed baseline/ry_20260417.sql
var Baseline string

//go:embed migrations/001_foundation.sql
var Foundation string

//go:embed migrations/002_assets.sql
var Assets string

//go:embed migrations/003_asset_node_identity.sql
var AssetNodeIdentity string

//go:embed migrations/004_execution.sql
var Execution string

//go:embed migrations/005_qwen.sql
var Qwen string

//go:embed migrations/006_templates.sql
var Templates string

//go:embed migrations/007_batches.sql
var Batches string

//go:embed migrations/008_admin_foundation.sql
var AdminFoundation string

//go:embed migrations/009_model_catalog.sql
var ModelCatalog string

//go:embed migrations/010_creator_points.sql
var CreatorPoints string

//go:embed migrations/011_managed_execution.sql
var ManagedExecution string

//go:embed migrations/012_official_templates.sql
var OfficialTemplates string

//go:embed migrations/013_product_studio.sql
var ProductStudio string

//go:embed migrations/014_product_finals.sql
var ProductFinals string

//go:embed migrations/015_product_workflows.sql
var ProductWorkflows string

const ProductWorkflowsVersion = "015"

func ProductWorkflowsChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(ProductWorkflows, "\r\n", "\n"))))
}

const ProductFinalsVersion = "014"

func ProductFinalsChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(ProductFinals, "\r\n", "\n"))))
}

const ProductStudioVersion = "013"

func ProductStudioChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(ProductStudio, "\r\n", "\n"))))
}

const OfficialTemplatesVersion = "012"

func OfficialTemplatesChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(OfficialTemplates, "\r\n", "\n"))))
}

const ManagedExecutionVersion = "011"

func ManagedExecutionChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(ManagedExecution, "\r\n", "\n"))))
}

const CreatorPointsVersion = "010"

func CreatorPointsChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(CreatorPoints, "\r\n", "\n"))))
}

const ModelCatalogVersion = "009"

func ModelCatalogChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(ModelCatalog, "\r\n", "\n"))))
}

const AdminFoundationVersion = "008"

func AdminFoundationChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(AdminFoundation, "\r\n", "\n"))))
}

const BatchesVersion = "007"

func BatchesChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Batches, "\r\n", "\n"))))
}

const TemplatesVersion = "006"

func TemplatesChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Templates, "\r\n", "\n"))))
}

const QwenVersion = "005"

func QwenChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Qwen, "\r\n", "\n"))))
}

const ExecutionVersion = "004"

func ExecutionChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Execution, "\r\n", "\n"))))
}

const AssetNodeIdentityVersion = "003"

func AssetNodeIdentityChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(AssetNodeIdentity, "\r\n", "\n"))))
}

const AssetsVersion = "002"

func AssetsChecksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Assets, "\r\n", "\n"))))
}

const Version = "001"
const BaselineSHA256 = "09f26cb52f2768e87f3860200e54ca39a491c373b766ef7cf404c2683bd7ae0b"

func Checksum() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(Baseline+"\n"+Foundation, "\r\n", "\n"))))
}
