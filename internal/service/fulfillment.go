package service

import (
	"context"
	"regexp"
	"ruoyi-go/internal/model"
	"ruoyi-go/internal/repository"
	"ruoyi-go/pkg/errs"
	"ruoyi-go/pkg/page"
)

var carrierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validERPAction(in model.ERPActionToken) error {
	if in.Version < 1 || !orderRequestKeyPattern.MatchString(in.RequestKey) {
		return errs.New("动作须带正数版本和8—64位请求标识")
	}
	return nil
}
func ReviewERPOrder(ctx context.Context, id int64, in model.ERPReviewInput, operator string) (*model.ERPFulfillmentResult, error) {
	if err := validERPAction(in.ERPActionToken); err != nil {
		return nil, err
	}
	if id < 1 || in.WarehouseID < 1 {
		return nil, errs.New("订单/仓库编号不正确")
	}
	return repository.ReviewERPOrder(ctx, id, in, orderHash([]any{"REVIEW", id, in}), operator)
}
func CancelERPOrder(ctx context.Context, id int64, in model.ERPCancelInput, operator string) (*model.ERPFulfillmentResult, error) {
	if err := validERPAction(in.ERPActionToken); err != nil {
		return nil, err
	}
	if id < 1 || !orderText(in.Reason, 500, true) {
		return nil, errs.New("取消须填写1—500字原因，不能含首尾空白或控制字符")
	}
	return repository.CancelERPOrder(ctx, id, in, orderHash([]any{"CANCEL", id, in}), operator)
}
func SetERPPackageLogistics(ctx context.Context, id int64, in model.ERPLogisticsInput, operator string) (*model.ERPFulfillmentResult, error) {
	if err := validERPAction(in.ERPActionToken); err != nil {
		return nil, err
	}
	if id < 1 || !carrierPattern.MatchString(in.CarrierCode) || !orderText(in.TrackingNo, 128, true) {
		return nil, errs.New("物流代码须为1—64位字母数字_或-，运单号须为1—128字文本")
	}
	return repository.ActERPPackage(ctx, id, in, false, orderHash([]any{"LOGISTICS", id, in}), operator)
}
func ShipERPPackage(ctx context.Context, id int64, in model.ERPActionToken, operator string) (*model.ERPFulfillmentResult, error) {
	if err := validERPAction(in); err != nil {
		return nil, err
	}
	if id < 1 {
		return nil, errs.New("包裹编号不正确")
	}
	return repository.ActERPPackage(ctx, id, model.ERPLogisticsInput{ERPActionToken: in}, true, orderHash([]any{"SHIP", id, in}), operator)
}
func ListERPPackages(ctx context.Context, q model.ERPPackageQuery, pg page.Query) ([]model.ERPPackageSummary, int64, error) {
	switch q.Status {
	case "", "READY_TO_SHIP", "SHIPPED", "CANCELLED":
	default:
		return nil, 0, errs.New("包裹状态不正确")
	}
	return repository.SelectERPPackages(ctx, q, pg)
}
func GetERPPackage(ctx context.Context, id int64) (*model.ERPPackage, error) {
	return repository.SelectERPPackage(ctx, id)
}
