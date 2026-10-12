package seed

import (
	"catcare/internal/model"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"time"
)

func ptr[T any](v T) *T { return &v }
func Run(db *gorm.DB, password string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var u model.User
		err := tx.First(&u, "username = ?", "demo").Error
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
		u = model.User{Username: "demo", DisplayName: "猫猫的家人", PasswordHash: string(hash)}
		if err = tx.Create(&u).Error; err != nil {
			return err
		}
		t := model.Table{OwnerID: u.ID, Name: "猫猫的生活手账", Description: "记录每一个小小变化，一起照顾毛茸茸的家人。", Visibility: "private"}
		if err = tx.Create(&t).Error; err != nil {
			return err
		}
		cats := []model.Cat{{TableID: t.ID, Name: "糯米", Breed: "英国短毛猫", Sex: "male", Birthday: "2023-04-08", Color: "sage", Neutered: true}, {TableID: t.ID, Name: "桃酥", Breed: "中华田园猫", Sex: "female", Birthday: "2024-06-12", Color: "peach", Neutered: true}, {TableID: t.ID, Name: "六六", Breed: "布偶猫", Sex: "male", Birthday: "2022-11-06", Color: "lavender", Neutered: true}}
		if err = tx.Create(&cats).Error; err != nil {
			return err
		}
		ls := []model.Litter{{TableID: t.ID, Name: "原味混合猫砂", Material: "mixed", WaitMinutes: 10, Notes: "同一把铲子，轻抖后称重", Version: 1}, {TableID: t.ID, Name: "细颗粒豆腐砂", Material: "tofu", WaitMinutes: 10, Version: 1}}
		if err = tx.Create(&ls).Error; err != nil {
			return err
		}
		bs := []model.Box{{TableID: t.ID, Name: "客厅的共享盆", Location: "客厅 · 窗边", LitterID: ls[0].ID, CatIDs: []uint{cats[0].ID, cats[1].ID}}, {TableID: t.ID, Name: "阳台猫砂盆", Location: "阳台 · 安静角落", LitterID: ls[1].ID, CatIDs: []uint{cats[2].ID}}}
		if err = tx.Create(&bs).Error; err != nil {
			return err
		}
		now := time.Now().Add(-20 * time.Minute)
		rs := []model.Record{}
		ws := []model.Weight{}
		for day := 27; day >= 0; day-- {
			for ci, cat := range cats {
				b := bs[0]
				l := ls[0]
				if ci == 2 {
					b = bs[1]
					l = ls[1]
				}
				at := now.AddDate(0, 0, -day).Add(-time.Duration(ci) * time.Hour)
				g := 42.0 + float64(ci)*14 + float64(day%5-2)*2
				d := 4.5 + float64(ci)*0.7 + float64(day%3)*0.1
				loose := 2
				if day < 3 && ci == 1 {
					g = 25 + float64(day)
					d = 3.1
				}
				cid := cat.ID
				rs = append(rs, model.Record{TableID: t.ID, CreatorID: u.ID, Kind: "urine", CatID: &cid, BoxID: b.ID, OccurredAt: at, Count: 1, Grams: ptr(g), DiameterCM: ptr(d), Looseness: &loose, LitterKey: fmt.Sprintf("%d:v1", l.ID), LitterName: l.Name, WaitMinutes: l.WaitMinutes, EligibleCatIDs: b.CatIDs})
				rs = append(rs, model.Record{TableID: t.ID, CreatorID: u.ID, Kind: "stool", CatID: ptr(cat.ID), BoxID: b.ID, OccurredAt: at.Add(-2 * time.Hour), Count: 1, Grams: ptr(18.0 + float64(ci)*4), Moisture: ptr(3), Shape: "formed", Size: ptr(3), LitterKey: fmt.Sprintf("%d:v1", l.ID), LitterName: l.Name, WaitMinutes: l.WaitMinutes, EligibleCatIDs: b.CatIDs})
				if day%3 == 0 {
					kg := 4.32 + float64(ci)*0.55 + float64((27-day)%5)*0.015
					ws = append(ws, model.Weight{TableID: t.ID, CreatorID: u.ID, CatID: cat.ID, KG: kg, OccurredAt: at})
				}
			}
		}
		rs = append(rs, model.Record{TableID: t.ID, CreatorID: u.ID, Kind: "urine", BoxID: bs[0].ID, OccurredAt: now.Add(-4 * time.Hour), Count: 1, Grams: ptr(45.0), DiameterCM: ptr(4.6), Looseness: ptr(2), LitterKey: fmt.Sprintf("%d:v1", ls[0].ID), LitterName: ls[0].Name, WaitMinutes: 10, EligibleCatIDs: bs[0].CatIDs, Notes: "铲砂时发现，未目击使用"})
		if err = tx.Create(&rs).Error; err != nil {
			return err
		}
		if err = tx.Create(&ws).Error; err != nil {
			return err
		}
		public := model.Table{OwnerID: u.ID, Name: "一起养猫 · 公开交流表", Description: "公开示例表。登录后可申请协作，表主审核后共同记录。", Visibility: "public"}
		return tx.Create(&public).Error
	})
}
