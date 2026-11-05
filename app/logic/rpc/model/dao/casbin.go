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

// casbinPolicyColumns are the value columns of casbin_rule in Casbin's field
// order. RemoveFilteredPolicy indexes into this slice.
var casbinPolicyColumns = [...]string{"v0", "v1", "v2", "v3", "v4", "v5"}

// The persist.Adapter methods below turn *Store into Casbin's policy store.
//
// Only grouping (g) rules are persisted: the static policy (p) rules are seeded
// in memory at startup, so this table holds role assignments alone.

// LoadPolicy reads every rule into the model.
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

// SavePolicy is a no-op: the model is never rewritten wholesale, and a partial
// save would drop rows the caller did not build. Callers mutate rules through
// the incremental Add/Remove methods instead.
func (s *Store) SavePolicy(casbinmodel.Model) error { return nil }

// AddPolicy inserts one rule; duplicates are ignored so a repeated assignment
// stays idempotent.
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

// RemovePolicy deletes the exact rule.
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

// casbinQuery builds the equality filter for one rule, padding short rules so a
// g rule of three values still matches its stored row.
func (s *Store) casbinQuery(ptype string, rule []string) *gorm.DB {
	query := s.db.WithContext(context.Background()).Where("ptype = ?", ptype)
	for i, value := range trimTrailingEmpty(rule) {
		query = query.Where(casbinPolicyColumns[i]+" = ?", value)
	}
	return query
}

// casbinRuleRow maps a Casbin rule onto its column layout.
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
