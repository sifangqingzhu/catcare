package main

import (
	"catcare/internal/api"
	"catcare/internal/config"
	"catcare/internal/model"
	"catcare/internal/seed"
	"context"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, e := config.Load()
	if e != nil {
		log.Fatal(e)
	}
	db, e := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{TranslateError: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		log.Fatal("数据库连接失败，请检查 .env 和数据库健康状态")
	}
	sqlDB, e := db.DB()
	if e != nil {
		log.Fatal("数据库初始化失败")
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(15)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if e = db.AutoMigrate(&model.User{}, &model.Session{}, &model.Table{}, &model.Member{}, &model.Cat{}, &model.Litter{}, &model.Box{}, &model.Record{}, &model.Weight{}); e != nil {
		log.Fatal("数据库迁移失败")
	}
	if cfg.BootstrapUsername != "" {
		if e = seed.Owner(db, cfg.BootstrapUsername, cfg.BootstrapPassword); e != nil {
			log.Fatal("初始账户创建失败")
		}
	}
	if cfg.Demo {
		if e = seed.Run(db, cfg.DemoPassword); e != nil {
			log.Fatal("演示数据初始化失败")
		}
	}
	appCtx, stopAnalysis := context.WithCancel(context.Background())
	defer stopAnalysis()
	srv := &http.Server{Addr: cfg.Address, Handler: api.NewWithContext(appCtx, db, cfg), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		log.Printf("Catcare listening on %s", cfg.Address)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Fatal("服务启动失败")
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	stopAnalysis()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
