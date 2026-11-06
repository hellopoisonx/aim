package dao

import (
	"context"
	"errors"
	"fmt"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// casbinPolicyColumns are the value columns of casbin_rule in Casbin's field order.
var casbinPolicyColumns = [...]string{"v0", "v1", "v2", "v3", "v4", "v5"}

func (s *Store) LoadPolicy(m casbinmodel.Model) error {
	if s == nil || s.db == nil {
		return errors.New("logic dao: database is not configured")
	}

	var rows []orm.CasbinRule
	if err := s.db.WithContext(context.Background()).Order("id").Find(&rows).Error; err != nil {
		return fmt.Errorf("logic dao: load casbin policy: %w", err)
	}

	for _, row := range rows {
		rule := append([]string{row.Ptype}, trimTrailingEmpty(row.Values())...)
		if err := persist.LoadPolicyArray(rule, m); err != nil {
			return err
		}
	}

	return nil
}

// SavePolicy is unsupported: the static matrix is memory-only and the g rules are
// written incrementally, so a wholesale save would drop rows the caller did not build.
func (s *Store) SavePolicy(casbinmodel.Model) error {
	return errors.New("logic dao: SavePolicy is unsupported; mutate rules through AddPolicy/RemovePolicy")
}

func (s *Store) AddPolicy(_ string, ptype string, rule []string) error {
	if s == nil || s.db == nil {
		return errors.New("logic dao: database is not configured")
	}

	row := casbinRuleRow(ptype, rule)
	if err := s.db.WithContext(context.Background()).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return fmt.Errorf("logic dao: add casbin policy: %w", err)
	}

	return nil
}

func (s *Store) RemovePolicy(_ string, ptype string, rule []string) error {
	if s == nil || s.db == nil {
		return errors.New("logic dao: database is not configured")
	}

	if err := s.casbinQuery(ptype, rule).Delete(&orm.CasbinRule{}).Error; err != nil {
		return fmt.Errorf("logic dao: remove casbin policy: %w", err)
	}

	return nil
}

// RemoveFilteredPolicy deletes the rules matching fieldValues, which start at
// fieldIndex in the v0..v5 column order. Empty values match any column.
func (s *Store) RemoveFilteredPolicy(_ string, ptype string, fieldIndex int, fieldValues ...string) error {
	if s == nil || s.db == nil {
		return errors.New("logic dao: database is not configured")
	}
	if fieldIndex < 0 || fieldIndex > len(casbinPolicyColumns) {
		return fmt.Errorf("logic dao: casbin field index %d out of range", fieldIndex)
	}

	query := s.db.WithContext(context.Background()).Where("ptype = ?", ptype)
	for i, value := range fieldValues {
		column := fieldIndex + i
		if value == "" {
			continue
		}
		if column >= len(casbinPolicyColumns) {
			return fmt.Errorf("logic dao: casbin field index %d out of range", column)
		}
		query = query.Where(casbinPolicyColumns[column]+" = ?", value)
	}

	if err := query.Delete(&orm.CasbinRule{}).Error; err != nil {
		return fmt.Errorf("logic dao: remove filtered casbin policy: %w", err)
	}

	return nil
}

func (s *Store) casbinQuery(ptype string, rule []string) *gorm.DB {
	query := s.db.WithContext(context.Background()).Where("ptype = ?", ptype)
	for i, value := range trimTrailingEmpty(rule) {
		query = query.Where(casbinPolicyColumns[i]+" = ?", value)
	}
	return query
}

func casbinRuleRow(ptype string, rule []string) orm.CasbinRule {
	values := trimTrailingEmpty(rule)
	row := orm.CasbinRule{Ptype: ptype}
	fields := []*string{&row.V0, &row.V1, &row.V2, &row.V3, &row.V4, &row.V5}
	for i, value := range values {
		*fields[i] = value
	}
	return row
}

// trimTrailingEmpty drops the unused tail of a rule: casbin_rule always has six
// value columns, but a g rule carries three.
func trimTrailingEmpty(values []string) []string {
	if len(values) > 6 {
		values = values[:6]
	}
	end := len(values)
	for end > 0 && values[end-1] == "" {
		end--
	}
	return values[:end]
}
