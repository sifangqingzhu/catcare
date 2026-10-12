package seed

import (
	"catcare/internal/model"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"regexp"
	"strings"
)

// Owner is an optional, create-only bootstrap. Existing passwords are never reset.
func Owner(db *gorm.DB, name, password string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if !regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`).MatchString(name) {
		return fmt.Errorf("invalid bootstrap username")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var u model.User
		err := tx.First(&u, "username = ?", name).Error
		if err == nil {
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u = model.User{Username: name, DisplayName: name, PasswordHash: string(hash)}
		if err = tx.Create(&u).Error; err != nil {
			return err
		}
		return tx.Create(&model.Table{OwnerID: u.ID, Name: "猫猫的生活手账", Description: "和家人一起记录猫咪的每个小变化。", Visibility: "private"}).Error
	})
}
