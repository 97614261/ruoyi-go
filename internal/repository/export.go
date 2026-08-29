package repository

import "gorm.io/gorm"

func applyOptionalLimit(db *gorm.DB, limits []int) *gorm.DB {
	if len(limits) > 0 && limits[0] > 0 {
		return db.Limit(limits[0])
	}
	return db
}
