package model

type CatalogCategory struct {
	ERPRecord
	CategoryID   int64  `gorm:"column:category_id;primaryKey;autoIncrement" json:"categoryId"`
	CategoryCode string `gorm:"column:category_code" json:"categoryCode"`
	CategoryName string `gorm:"column:category_name" json:"categoryName"`
	Status       string `gorm:"column:status" json:"status"`
	Version      int64  `gorm:"column:version" json:"version"`
}

func (CatalogCategory) TableName() string { return "erp_product_category" }

type CatalogProduct struct {
	ERPRecord
	ProductID    int64                `gorm:"column:product_id;primaryKey;autoIncrement" json:"productId"`
	ProductCode  string               `gorm:"column:product_code" json:"productCode"`
	ProductName  string               `gorm:"column:product_name" json:"productName"`
	CategoryID   *int64               `gorm:"column:category_id" json:"categoryId"`
	Version      int64                `gorm:"column:version" json:"version"`
	CategoryName string               `gorm:"-" json:"categoryName"`
	Variants     []CatalogVariantView `gorm:"-" json:"variants,omitempty"`
	Images       []string             `gorm:"-" json:"images"`
}

func (CatalogProduct) TableName() string { return "erp_product" }

type CatalogProfile struct {
	ERPRecord
	ProfileID   int64   `gorm:"column:profile_id;primaryKey;autoIncrement" json:"-"`
	ProductID   int64   `gorm:"column:product_id" json:"productId"`
	SkuID       int64   `gorm:"column:sku_id" json:"skuId"`
	Barcode     *string `gorm:"column:barcode" json:"barcode"`
	WeightGrams *int64  `gorm:"column:weight_grams" json:"weightGrams"`
	LengthMM    *int64  `gorm:"column:length_mm" json:"lengthMm"`
	WidthMM     *int64  `gorm:"column:width_mm" json:"widthMm"`
	HeightMM    *int64  `gorm:"column:height_mm" json:"heightMm"`
}

func (CatalogProfile) TableName() string { return "erp_sku_profile" }

type CatalogImage struct {
	ERPRecord
	ImageID    int64  `gorm:"column:image_id;primaryKey;autoIncrement" json:"-"`
	ProductID  int64  `gorm:"column:product_id" json:"productId"`
	StorageKey string `gorm:"column:storage_key" json:"storageKey"`
	Position   int    `gorm:"column:position" json:"position"`
}

func (CatalogImage) TableName() string { return "erp_product_image" }

type CatalogAction struct {
	ERPRecord
	ActionID    int64  `gorm:"column:action_id;primaryKey;autoIncrement" json:"-"`
	RequestKey  string `gorm:"column:request_key" json:"-"`
	RequestHash string `gorm:"column:request_hash" json:"-"`
	ActorID     int64  `gorm:"column:actor_id" json:"-"`
	ActionKind  string `gorm:"column:action_kind" json:"-"`
	ResultJSON  string `gorm:"column:result_json" json:"-"`
}

func (CatalogAction) TableName() string { return "erp_catalog_action" }

type CatalogVariant struct {
	SkuID         int64   `json:"skuId"`
	SkuCode       string  `json:"skuCode"`
	Specification string  `json:"specification"`
	Unit          string  `json:"unit"`
	Status        string  `json:"status"`
	Barcode       *string `json:"barcode"`
	WeightGrams   *int64  `json:"weightGrams"`
	LengthMM      *int64  `json:"lengthMm"`
	WidthMM       *int64  `json:"widthMm"`
	HeightMM      *int64  `json:"heightMm"`
}
type CatalogVariantView struct {
	CatalogVariant
	Referenced bool `json:"referenced"`
}
type CatalogWrite struct {
	RequestKey  string           `json:"requestKey"`
	Version     int64            `json:"version"`
	ProductCode string           `json:"productCode"`
	ProductName string           `json:"productName"`
	CategoryID  *int64           `json:"categoryId"`
	Remark      string           `json:"remark"`
	Variants    []CatalogVariant `json:"variants"`
	Images      []string         `json:"images"`
}
type CatalogCategoryWrite struct {
	RequestKey   string `json:"requestKey"`
	Version      int64  `json:"version"`
	CategoryCode string `json:"categoryCode"`
	CategoryName string `json:"categoryName"`
	Status       string `json:"status"`
	Remark       string `json:"remark"`
}
type CatalogResult struct {
	ProductIDs []int64 `json:"productIds"`
	CategoryID int64   `json:"categoryId,omitempty"`
	Version    int64   `json:"version"`
	Rows       int     `json:"rows"`
	Existing   bool    `json:"existing"`
}
type CatalogQuery struct {
	ProductCode  string `form:"productCode"`
	ProductName  string `form:"productName"`
	SkuCode      string `form:"skuCode"`
	Barcode      string `form:"barcode"`
	CategoryID   int64  `form:"categoryId"`
	CategoryCode string `form:"categoryCode"`
	CategoryName string `form:"categoryName"`
}
type CatalogExcelRow struct {
	ProductCode   string `excel:"name:商品编码" json:"productCode"`
	ProductName   string `excel:"name:商品名称" json:"productName"`
	CategoryCode  string `excel:"name:分类编码" json:"categoryCode"`
	SkuCode       string `excel:"name:SKU编码" json:"skuCode"`
	Specification string `excel:"name:规格" json:"specification"`
	Unit          string `excel:"name:单位" json:"unit"`
	Status        string `excel:"name:状态;readConverterExp:0=启用,1=停用" json:"status"`
	Barcode       string `excel:"name:条码" json:"barcode"`
	WeightGrams   string `excel:"name:重量克" json:"weightGrams"`
	LengthMM      string `excel:"name:长毫米" json:"lengthMm"`
	WidthMM       string `excel:"name:宽毫米" json:"widthMm"`
	HeightMM      string `excel:"name:高毫米" json:"heightMm"`
	Remark        string `excel:"name:备注" json:"remark"`
}
type CatalogImport struct {
	RequestKey string         `json:"requestKey"`
	Products   []CatalogWrite `json:"products"`
}
