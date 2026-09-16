package service

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ruoyi-go/internal/model"
)

func TestRenderGeneratedFilesProducesFormattedGoAndFrontend(t *testing.T) {
	formCols, one, two := 2, 1, 2
	genPath := "/"
	table := &model.GenTable{
		TableID: 1, TableName: "demo_order", TableComment: "订单表", ClassName: "DemoOrder",
		TplCategory: "crud", TplWebType: "element-plus", PackageName: "ruoyi-go",
		ModuleName: "demo", BusinessName: "order", FunctionName: "订单", FunctionAuthor: "tester",
		FormColNum: &formCols, GenType: "0", GenPath: &genPath, Params: map[string]any{"parentMenuId": 3},
		Columns: []model.GenTableColumn{
			{ColumnID: 1, TableID: 1, ColumnName: "order_id", ColumnComment: "订单编号", ColumnType: "bigint", JavaType: "Long", JavaField: "orderId", IsPK: "1", IsIncrement: "1", IsInsert: "1", IsEdit: "0", IsList: "1", IsQuery: "0", IsRequired: "0", QueryType: "EQ", HTMLType: "input", Sort: &one},
			{ColumnID: 2, TableID: 1, ColumnName: "order_name", ColumnComment: "订单名称", ColumnType: "varchar(100)", JavaType: "String", JavaField: "orderName", IsPK: "0", IsIncrement: "0", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", IsRequired: "1", QueryType: "LIKE", HTMLType: "input", Sort: &two},
		},
	}
	table.PKColumn = &table.Columns[0]
	files, err := renderGeneratedFiles(table)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 9 {
		t.Fatalf("expected 9 generated files, got %d", len(files))
	}
	seen := map[string]bool{}
	for _, file := range files {
		if seen[file.Path] || file.Content == "" {
			t.Fatalf("invalid generated file: %#v", file)
		}
		seen[file.Path] = true
		if strings.HasSuffix(file.Path, ".go") {
			if _, err := parser.ParseFile(token.NewFileSet(), file.Path, file.Content, parser.AllErrors); err != nil {
				t.Fatalf("generated %s is not valid Go: %v\n%s", file.Path, err, file.Content)
			}
		}
	}
	if !strings.Contains(files[0].PreviewKey, "domain.java.vm") {
		t.Fatalf("first preview key must keep Vue3 default tab compatible: %s", files[0].PreviewKey)
	}
	if !strings.Contains(findGeneratedContent(files, "sql/orderMenu.sql"), "demo:order:list") {
		t.Fatal("menu SQL missed permission prefix")
	}
	for label, pair := range map[string][2]string{
		"model excel tag":    {findGeneratedContent(files, "internal/model/order.go"), `excel:"name:订单名称"`},
		"repository export":  {findGeneratedContent(files, "internal/repository/order.go"), "SelectDemoOrderList"},
		"service export cap": {findGeneratedContent(files, "internal/service/order.go"), "MaxExportRows+1"},
		"handler export":     {findGeneratedContent(files, "internal/handler/order.go"), "excelx.WriteResponse"},
		"router export":      {findGeneratedContent(files, "internal/router/order_routes.go"), "middleware.ExportLimit()"},
		"frontend export":    {findGeneratedContent(files, "vue/src/views/demo/order/index.vue"), "handleExport"},
		"menu export":        {findGeneratedContent(files, "sql/orderMenu.sql"), "demo:order:export"},
	} {
		if !strings.Contains(pair[0], pair[1]) {
			t.Fatalf("%s missing %q", label, pair[1])
		}
	}
	if !strings.Contains(findGeneratedContent(files, "internal/model/order.go"), `binding:"required"`) {
		t.Fatal("generated model missed server-side required validation")
	}
	if !strings.Contains(findGeneratedContent(files, "internal/service/order.go"), "target.OrderId = 0") {
		t.Fatal("generated service must ignore client supplied auto-increment primary key")
	}
}

func TestPathWithin(t *testing.T) {
	base := filepath.Join("root", "generated")
	if !pathWithin(base, filepath.Join(base, "internal", "model.go")) {
		t.Fatal("child path should be accepted")
	}
	if pathWithin(base, filepath.Join(base, "..", "escape.go")) {
		t.Fatal("parent escape should be rejected")
	}
}

func TestWriteGeneratedFileSafelyOverwritesRegularFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "nested", "generated.go")
	if err := writeGeneratedFile(target, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := writeGeneratedFile(target, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("generated file = %q, want new", data)
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".ruoyi-gen-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files leaked: %v, err=%v", matches, err)
	}
}

func TestRenderGeneratedFilesSupportsTreeAndWebVariants(t *testing.T) {
	table := sampleRenderTable()
	table.TplCategory = "tree"
	table.TreeCode, table.TreeParentCode, table.TreeName = "order_id", "parent_id", "order_name"
	table.Params = map[string]any{"treeCode": "order_id", "treeParentCode": "parent_id", "treeName": "order_name"}
	table.Columns = append(table.Columns, model.GenTableColumn{ColumnID: 3, ColumnName: "parent_id", ColumnComment: "上级", JavaType: "Long", JavaField: "parentId", IsInsert: "1", IsEdit: "1", IsList: "1", QueryType: "EQ", HTMLType: "input"})
	for _, webType := range []string{"element-plus", "element-plus-typescript", "element-ui"} {
		table.TplWebType = webType
		files, err := renderGeneratedFiles(table)
		if err != nil {
			t.Fatalf("%s: %v", webType, err)
		}
		vue := findGeneratedContent(files, "vue/src/views/demo/order/index.vue")
		if !strings.Contains(vue, "all.length < count") || !strings.Contains(vue, "row-key=\"orderId\"") {
			t.Fatalf("%s tree output can truncate data:\n%s", webType, vue)
		}
		if !strings.Contains(vue, "handleAdd(scope.row)") || !strings.Contains(vue, "makeTreeOptions") ||
			!strings.Contains(vue, "顶级节点") {
			t.Fatalf("%s tree output missed child creation or parent options:\n%s", webType, vue)
		}
		repositoryCode := findGeneratedContent(files, "internal/repository/order.go")
		serviceCode := findGeneratedContent(files, "internal/service/order.go")
		if !strings.Contains(repositoryCode, "func ValidateDemoOrderParent") ||
			!strings.Contains(repositoryCode, "上级节点不能是当前节点或其子节点") ||
			!strings.Contains(repositoryCode, "Find(&rows)") ||
			strings.Contains(repositoryCode, "Take(&row)") ||
			!strings.Contains(serviceCode, "repository.ValidateDemoOrderParent") {
			t.Fatalf("%s tree output missed server-side cycle validation", webType)
		}
		if webType == "element-ui" {
			if !strings.Contains(vue, "<el-cascader") {
				t.Fatal("Vue2 tree output missed parent selector")
			}
		} else if !strings.Contains(vue, "<el-tree-select") {
			t.Fatal("Vue3 tree output missed parent selector")
		}
		apiPath := "vue/src/api/demo/order.js"
		if webType == "element-plus-typescript" {
			apiPath = "vue/src/api/demo/order.ts"
			if !strings.Contains(findGeneratedContent(files, apiPath), "Array<string | number>") {
				t.Fatal("typescript delete API must accept batch ids")
			}
			if !strings.Contains(vue, "ComponentInternalInstance") || !strings.Contains(vue, "selection: Array<Record<string, any>>") {
				t.Fatal("typescript page missed strict-mode annotations")
			}
		}
	}
}

func TestValidateTreeRenderRequiresCompatibleIdentifierTypes(t *testing.T) {
	for _, parentType := range []string{"String", "Date"} {
		t.Run(parentType, func(t *testing.T) {
			table := sampleRenderTable()
			table.TplCategory = "tree"
			table.TreeCode, table.TreeParentCode, table.TreeName = "order_id", "parent_id", "order_name"
			table.Params = map[string]any{"treeCode": "order_id", "treeParentCode": "parent_id", "treeName": "order_name"}
			table.Columns = append(table.Columns, model.GenTableColumn{
				ColumnID: 3, ColumnName: "parent_id", JavaType: parentType, JavaField: "parentId",
				IsInsert: "1", IsEdit: "1", QueryType: "EQ", HTMLType: "input",
			})
			if err := validateGenTableForRender(table); err == nil || !strings.Contains(err.Error(), "相同") {
				t.Fatalf("expected incompatible tree identifier type %s to fail, got %v", parentType, err)
			}
		})
	}
}

func TestRenderGeneratedFilesSupportsTransactionalSubTable(t *testing.T) {
	table := sampleRenderTable()
	table.TplCategory = "sub"
	subName, fkName := "demo_order_item", "order_id"
	table.SubTableName, table.SubTableFKName = &subName, &fkName
	table.SubTable = &model.GenTable{
		TableName: "demo_order_item", TableComment: "订单明细", ClassName: "DemoOrderItem", FunctionName: "订单明细",
		Columns: []model.GenTableColumn{
			{ColumnName: "item_id", ColumnComment: "明细编号", JavaType: "Long", JavaField: "itemId", IsPK: "1", IsIncrement: "1", QueryType: "EQ", HTMLType: "input"},
			{ColumnName: "order_id", ColumnComment: "订单编号", JavaType: "Long", JavaField: "orderId", IsInsert: "1", IsEdit: "1", QueryType: "EQ", HTMLType: "input"},
			{ColumnName: "product_name", ColumnComment: "商品", JavaType: "String", JavaField: "productName", IsInsert: "1", IsEdit: "1", QueryType: "EQ", HTMLType: "input"},
		},
	}
	setGenPKColumn(table)
	files, err := renderGeneratedFiles(table)
	if err != nil {
		t.Fatal(err)
	}
	modelCode := findGeneratedContent(files, "internal/model/order.go")
	repoCode := findGeneratedContent(files, "internal/repository/order.go")
	vue := findGeneratedContent(files, "vue/src/views/demo/order/index.vue")
	for label, pair := range map[string][2]string{
		"model child list":         {modelCode, "DemoOrderItemList []DemoOrderItem"},
		"repository transaction":   {repoCode, "return Transaction(ctx"},
		"repository child replace": {repoCode, "清理子表数据失败"},
		"frontend child editor":    {vue, "addSubRow"},
	} {
		if !strings.Contains(pair[0], pair[1]) {
			t.Fatalf("%s missing %q", label, pair[1])
		}
	}
}

func sampleRenderTable() *model.GenTable {
	formCols, one, two := 2, 1, 2
	genPath := "/"
	table := &model.GenTable{
		TableID: 1, TableName: "demo_order", TableComment: "订单表", ClassName: "DemoOrder",
		TplCategory: "crud", TplWebType: "element-plus", PackageName: "ruoyi-go",
		ModuleName: "demo", BusinessName: "order", FunctionName: "订单", FunctionAuthor: "tester",
		FormColNum: &formCols, GenType: "0", GenPath: &genPath, Params: map[string]any{"parentMenuId": 3},
		Columns: []model.GenTableColumn{
			{ColumnID: 1, TableID: 1, ColumnName: "order_id", ColumnComment: "订单编号", ColumnType: "bigint", JavaType: "Long", JavaField: "orderId", IsPK: "1", IsIncrement: "1", IsInsert: "1", IsList: "1", QueryType: "EQ", HTMLType: "input", Sort: &one},
			{ColumnID: 2, TableID: 1, ColumnName: "order_name", ColumnComment: "订单名称", ColumnType: "varchar(100)", JavaType: "String", JavaField: "orderName", IsInsert: "1", IsEdit: "1", IsList: "1", IsQuery: "1", IsRequired: "1", QueryType: "LIKE", HTMLType: "input", Sort: &two},
		},
	}
	setGenPKColumn(table)
	return table
}

func findGeneratedContent(files []generatedFile, path string) string {
	for _, file := range files {
		if filepath.ToSlash(file.Path) == path {
			return file.Content
		}
	}
	return ""
}
