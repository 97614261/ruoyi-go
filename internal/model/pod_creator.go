package model

import "time"

// These DTOs intentionally exclude sys_user secrets and private creative content.
type PODCreatorPolicy struct {
	UserID              int64      `gorm:"primaryKey" json:"userId,string"`
	WorkspaceID         int64      `json:"workspaceId,string"`
	Status              string     `json:"status"`
	ValidUntil          *time.Time `json:"validUntil"`
	MaxConcurrentNodes  int        `json:"maxConcurrentNodes"`
	MaxPendingRuns      int        `json:"maxPendingRuns"`
	MaxDailySubmissions int        `json:"maxDailySubmissions"`
	MaxStorageBytes     int64      `json:"maxStorageBytes,string"`
	Revision            int64      `json:"revision,string"`
	LastDispatchedAt    *time.Time `json:"-"`
}

func (PODCreatorPolicy) TableName() string { return "pod_creator_policy" }

type PODPointsAccount struct {
	UserID        int64 `gorm:"primaryKey" json:"userId,string"`
	Available     int64 `json:"available,string"`
	Held          int64 `json:"held,string"`
	Spent         int64 `json:"spent,string"`
	GrantedTotal  int64 `json:"grantedTotal,string"`
	DeductedTotal int64 `json:"deductedTotal,string"`
	Revision      int64 `json:"revision,string"`
}

func (PODPointsAccount) TableName() string { return "pod_points_account" }

type PODStorageAccount struct {
	UserID    int64 `gorm:"primaryKey" json:"userId,string"`
	UsedBytes int64 `json:"usedBytes,string"`
	HeldBytes int64 `json:"heldBytes,string"`
	Revision  int64 `json:"revision,string"`
}

func (PODStorageAccount) TableName() string { return "pod_storage_account" }

type PODStorageReservation struct {
	ReservationID  int64 `gorm:"primaryKey;autoIncrement"`
	UserID         int64
	OperationKey   string
	ReservedBytes  int64
	CommittedBytes int64
	State          string
}

func (PODStorageReservation) TableName() string { return "pod_storage_reservation" }

type PODCreatorView struct {
	PODCreatorPolicy
	UserName      string `json:"userName"`
	NickName      string `json:"nickName"`
	AccountStatus string `json:"accountStatus"`
	Expired       bool   `json:"expired"`
}
type PODCreatorDetail struct {
	Creator          *PODCreatorView   `json:"creator"`
	ModelIDs         []string          `json:"modelIds"`
	Storage          PODStorageAccount `json:"storage"`
	DailySubmissions int               `json:"dailySubmissions"`
	BusinessDate     string            `json:"businessDate"`
}
type PODCreatorWrite struct {
	RequestID       string                 `json:"requestId"`
	Revision        string                 `json:"revision"`
	AccountRevision string                 `json:"accountRevision"`
	Reason          string                 `json:"reason"`
	UserName        string                 `json:"userName"`
	NickName        string                 `json:"nickName"`
	InitialPassword string                 `json:"initialPassword"`
	Policy          *PODCreatorPolicyInput `json:"policy"`
	ModelIDs        *[]string              `json:"modelIds"`
	Kind            string                 `json:"kind"`
	Amount          string                 `json:"amount"`
}
type PODCreatorPolicyInput struct {
	Status              string  `json:"status"`
	ValidUntil          *string `json:"validUntil"`
	MaxConcurrentNodes  int     `json:"maxConcurrentNodes"`
	MaxPendingRuns      int     `json:"maxPendingRuns"`
	MaxDailySubmissions int     `json:"maxDailySubmissions"`
	MaxStorageBytes     string  `json:"maxStorageBytes"`
}
type PODPointsEventView struct {
	EventID        int64     `json:"eventId,string"`
	UserID         int64     `json:"userId,string"`
	Kind           string    `json:"kind"`
	Amount         int64     `json:"amount,string"`
	AvailableAfter int64     `json:"availableAfter,string"`
	HeldAfter      int64     `json:"heldAfter,string"`
	SpentAfter     int64     `json:"spentAfter,string"`
	Reason         string    `json:"reason"`
	ActorUserID    int64     `json:"actorUserId,string"`
	CreateTime     time.Time `json:"createTime"`
}
type PODCreatorAuditSnapshot struct {
	Policy   *PODCreatorPolicy `json:"policy,omitempty"`
	ModelIDs []string          `json:"modelIds"`
	Points   *PODPointsAccount `json:"points,omitempty"`
}

type PODCreatorModelOption struct {
	ModelID        int64  `json:"modelId,string"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	ProviderStatus string `json:"providerStatus"`
}
