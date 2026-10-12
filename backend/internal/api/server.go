package api

import (
	"catcare/internal/analysis"
	"catcare/internal/config"
	"catcare/internal/model"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	DB       *gorm.DB
	Config   config.Config
	limiter  sync.Mutex
	attempts map[string][]time.Time
}

func New(db *gorm.DB, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.EnableJsonDecoderDisallowUnknownFields()
	s := &Server{DB: db, Config: cfg, attempts: map[string][]time.Time{}}
	r := gin.New()
	r.Use(gin.Recovery())
	_ = r.SetTrustedProxies(cfg.TrustedProxies)
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			origin := c.GetHeader("Origin")
			if (origin != "" && origin != cfg.Origin) || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
				fail(c, 403, "请求来源不允许")
				return
			}
		}
		c.Next()
	})
	r.GET("/api/capabilities", func(c *gin.Context) { c.JSON(200, gin.H{"demo": cfg.Demo}) })
	r.GET("/api/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			fail(c, 503, "数据库暂不可用")
			return
		}
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.POST("/api/auth/register", s.throttle, s.register)
	r.POST("/api/auth/login", s.throttle, s.login)
	r.POST("/api/auth/demo", s.throttle, func(c *gin.Context) {
		if !cfg.Demo {
			fail(c, 404, "演示入口未启用")
			return
		}
		s.authenticate(c, "demo", cfg.DemoPassword)
	})
	auth := r.Group("/api")
	auth.Use(s.session)
	auth.GET("/me", func(c *gin.Context) { c.JSON(200, gin.H{"user": c.MustGet("user"), "demo": cfg.Demo}) })
	auth.POST("/auth/logout", func(c *gin.Context) {
		token, _ := c.Cookie("catcare_session")
		if err := db.Delete(&model.Session{}, "token_hash = ?", s.hash(token)).Error; err != nil {
			fail(c, 500, "退出失败")
			return
		}
		s.cookie(c, "", -1)
		c.JSON(200, gin.H{"ok": true})
	})
	auth.GET("/tables", s.myTables)
	auth.POST("/tables", s.createTable)
	auth.GET("/public", s.publicTables)
	tables := auth.Group("/tables/:table")
	tables.Use(s.access)
	tables.GET("/data", s.data)
	tables.PATCH("", s.owner, s.updateTable)
	tables.POST("/join", s.join)
	tables.GET("/members", s.members)
	tables.POST("/members", s.owner, s.invite)
	tables.PATCH("/members/:user", s.owner, s.updateMember)
	write := tables.Group("")
	write.Use(s.editor)
	write.POST("/cats", s.saveCat)
	write.PATCH("/cats/:id", s.saveCat)
	write.POST("/litters", s.saveLitter)
	write.PATCH("/litters/:id", s.saveLitter)
	write.POST("/boxes", s.saveBox)
	write.PATCH("/boxes/:id", s.saveBox)
	write.POST("/records", s.saveRecord)
	write.POST("/batches", s.saveBatch)
	write.PATCH("/records/:id", s.saveRecord)
	write.DELETE("/records/:id", s.deleteRecord)
	write.POST("/weights", s.saveWeight)
	write.DELETE("/weights/:id", s.deleteWeight)
	if cfg.FrontendDir != "" {
		r.Static("/assets", filepath.Join(cfg.FrontendDir, "assets"))
		r.GET("/favicon.svg", func(c *gin.Context) { c.File(filepath.Join(cfg.FrontendDir, "favicon.svg")) })
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				fail(c, 404, "接口不存在")
				return
			}
			if c.Request.Method != "GET" {
				fail(c, 404, "页面不存在")
				return
			}
			if _, err := os.Stat(filepath.Join(cfg.FrontendDir, "index.html")); err != nil {
				fail(c, 404, "请先构建前端")
				return
			}
			c.File(filepath.Join(cfg.FrontendDir, "index.html"))
		})
	}
	return r
}
func fail(c *gin.Context, code int, msg string) { c.AbortWithStatusJSON(code, gin.H{"error": msg}) }
func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		fail(c, 400, "输入格式不正确，请检查必填项")
		return false
	}
	return true
}
func uid(c *gin.Context) uint { return c.MustGet("user").(model.User).ID }
func tid(c *gin.Context) uint { return c.MustGet("table").(model.Table).ID }
func idParam(c *gin.Context, name string) (uint, bool) {
	n, e := strconv.ParseUint(c.Param(name), 10, 32)
	if e != nil || n == 0 {
		fail(c, 400, "编号不正确")
		return 0, false
	}
	return uint(n), true
}
func (s *Server) hash(v string) string {
	mac := hmac.New(sha256.New, []byte(s.Config.Secret))
	mac.Write([]byte(v))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Server) cookie(c *gin.Context, value string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "catcare_session", Value: value, Path: "/", MaxAge: age, HttpOnly: true, Secure: s.Config.Secure, SameSite: http.SameSiteLaxMode})
}
func (s *Server) issue(c *gin.Context, u model.User) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		fail(c, 500, "登录失败")
		return
	}
	token := hex.EncodeToString(b)
	if e := s.DB.Create(&model.Session{TokenHash: s.hash(token), UserID: u.ID, ExpiresAt: time.Now().Add(24 * time.Hour)}).Error; e != nil {
		fail(c, 500, "登录失败")
		return
	}
	s.cookie(c, token, 86400)
	_ = s.DB.Where("expires_at < ?", time.Now()).Delete(&model.Session{}).Error
	c.JSON(200, gin.H{"user": u})
}
func (s *Server) session(c *gin.Context) {
	token, err := c.Cookie("catcare_session")
	if err != nil || len(token) != 64 {
		fail(c, 401, "请先登录")
		return
	}
	var ses model.Session
	if s.DB.First(&ses, "token_hash = ? AND expires_at > ?", s.hash(token), time.Now()).Error != nil {
		fail(c, 401, "登录已过期，请重新登录")
		return
	}
	var u model.User
	if s.DB.First(&u, ses.UserID).Error != nil {
		fail(c, 401, "账户不存在")
		return
	}
	c.Set("user", u)
	c.Next()
}
func (s *Server) throttle(c *gin.Context) {
	s.limiter.Lock()
	defer s.limiter.Unlock()
	now := time.Now()
	key := c.ClientIP()
	xs := s.attempts[key]
	keep := []time.Time{}
	for _, t := range xs {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= 15 {
		fail(c, 429, "尝试次数过多，请一分钟后再试")
		return
	}
	s.attempts[key] = append(keep, now)
	if len(s.attempts) > 5000 {
		for k, ts := range s.attempts {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > time.Minute {
				delete(s.attempts, k)
			}
		}
	}
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func (s *Server) register(c *gin.Context) {
	var in struct {
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"display_name" binding:"required"`
	}
	if !bind(c, &in) {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !usernamePattern.MatchString(in.Username) || len(in.Password) < 8 || len(in.Password) > 72 || in.DisplayName == "" || len([]rune(in.DisplayName)) > 40 {
		fail(c, 400, "账号须为3–32位英文、数字或下划线；密码为8–72字节；昵称最多40字")
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		fail(c, 500, "注册失败")
		return
	}
	u := model.User{Username: in.Username, DisplayName: in.DisplayName, PasswordHash: string(hash)}
	if e = s.DB.Create(&u).Error; e != nil {
		if errors.Is(e, gorm.ErrDuplicatedKey) {
			fail(c, 409, "账号已被使用")
		} else {
			fail(c, 500, "注册失败")
		}
		return
	}
	s.issue(c, u)
}
func (s *Server) login(c *gin.Context) {
	var in struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if !bind(c, &in) {
		return
	}
	s.authenticate(c, strings.ToLower(strings.TrimSpace(in.Username)), in.Password)
}
func (s *Server) authenticate(c *gin.Context, name, password string) {
	var u model.User
	err := s.DB.First(&u, "username = ?", name).Error
	if err != nil {
		_, _ = bcrypt.GenerateFromPassword([]byte("account-not-found-timing-padding"), bcrypt.DefaultCost)
		fail(c, 401, "账号或密码不正确")
		return
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		fail(c, 401, "账号或密码不正确")
		return
	}
	s.issue(c, u)
}
func (s *Server) access(c *gin.Context) {
	id, ok := idParam(c, "table")
	if !ok {
		return
	}
	var t model.Table
	if s.DB.First(&t, id).Error != nil {
		fail(c, 404, "表格不存在或无访问权限")
		return
	}
	role := "public"
	status := "none"
	if t.OwnerID == uid(c) {
		role = "owner"
		status = "approved"
	} else {
		var m model.Member
		err := s.DB.First(&m, "table_id = ? AND user_id = ?", id, uid(c)).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, 500, "权限查询失败")
			return
		}
		if err == nil {
			status = m.Status
			if m.Status == "approved" {
				role = m.Role
			}
		}
		if role == "public" && t.Visibility != "public" {
			fail(c, 404, "表格不存在或无访问权限")
			return
		}
	}
	c.Set("table", t)
	c.Set("role", role)
	c.Set("join_status", status)
	c.Next()
}
func (s *Server) owner(c *gin.Context) {
	if c.GetString("role") != "owner" {
		fail(c, 403, "只有表主可操作")
		return
	}
	c.Next()
}
func (s *Server) editor(c *gin.Context) {
	role := c.GetString("role")
	if role != "owner" && role != "editor" {
		fail(c, 403, "当前仅有浏览权限")
		return
	}
	c.Next()
}
func (s *Server) myTables(c *gin.Context) {
	var ts []model.Table
	if e := s.DB.Where("owner_id = ? OR id IN (?)", uid(c), s.DB.Model(&model.Member{}).Select("table_id").Where("user_id = ? AND status = ?", uid(c), "approved")).Order("id asc").Find(&ts).Error; e != nil {
		fail(c, 500, "加载失败")
		return
	}
	c.JSON(200, ts)
}
func (s *Server) publicTables(c *gin.Context) {
	var ts []model.Table
	q := s.DB.Where("visibility = ?", "public")
	if term := c.Query("q"); term != "" {
		q = q.Where("name ILIKE ?", "%"+term+"%")
	}
	if e := q.Order("id desc").Limit(100).Find(&ts).Error; e != nil {
		fail(c, 500, "加载失败")
		return
	}
	c.JSON(200, ts)
}

type tableInput struct {
	Name        string `json:"name" binding:"required,max=80"`
	Description string `json:"description" binding:"max=300"`
	Visibility  string `json:"visibility" binding:"required,oneof=public private"`
}

func (s *Server) createTable(c *gin.Context) {
	var in tableInput
	if !bind(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		fail(c, 400, "请输入表格名称")
		return
	}
	t := model.Table{OwnerID: uid(c), Name: in.Name, Description: in.Description, Visibility: in.Visibility}
	if e := s.DB.Create(&t).Error; e != nil {
		fail(c, 500, "创建失败")
		return
	}
	c.JSON(201, t)
}
func (s *Server) updateTable(c *gin.Context) {
	var in tableInput
	if !bind(c, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "请输入表格名称")
		return
	}
	if e := s.DB.Model(&model.Table{}).Where("id = ?", tid(c)).Updates(map[string]any{"name": strings.TrimSpace(in.Name), "description": in.Description, "visibility": in.Visibility}).Error; e != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) join(c *gin.Context) {
	t := c.MustGet("table").(model.Table)
	if t.Visibility != "public" {
		fail(c, 403, "仅可申请公开表格")
		return
	}
	if c.GetString("role") != "public" {
		c.JSON(200, gin.H{"status": "approved"})
		return
	}
	m := model.Member{TableID: t.ID, UserID: uid(c), Role: "viewer", Status: "pending"}
	if e := s.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "table_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"status": "pending", "role": "viewer"})}).Create(&m).Error; e != nil {
		fail(c, 500, "申请失败")
		return
	}
	c.JSON(200, m)
}
func (s *Server) members(c *gin.Context) {
	if c.GetString("role") == "public" {
		fail(c, 403, "加入后可查看成员")
		return
	}
	type item struct {
		UserID      uint   `json:"user_id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Status      string `json:"status"`
	}
	var ms []item
	q := s.DB.Table("members").Select("members.user_id, users.username, users.display_name, members.role, members.status").Joins("JOIN users ON users.id = members.user_id").Where("members.table_id = ?", tid(c))
	if c.GetString("role") != "owner" {
		q = q.Where("members.status = ?", "approved")
	}
	if e := q.Order("members.created_at").Scan(&ms).Error; e != nil {
		fail(c, 500, "加载失败")
		return
	}
	owner := c.MustGet("table").(model.Table).OwnerID
	var u model.User
	if s.DB.First(&u, owner).Error != nil {
		fail(c, 500, "加载失败")
		return
	}
	ms = append([]item{{u.ID, u.Username, u.DisplayName, "owner", "approved"}}, ms...)
	c.JSON(200, ms)
}
func (s *Server) invite(c *gin.Context) {
	var in struct {
		Username string `json:"username" binding:"required"`
		Role     string `json:"role" binding:"required,oneof=viewer editor"`
	}
	if !bind(c, &in) {
		return
	}
	var u model.User
	if s.DB.First(&u, "username = ?", strings.ToLower(strings.TrimSpace(in.Username))).Error != nil {
		fail(c, 404, "未找到此账号，请先注册")
		return
	}
	if u.ID == uid(c) {
		fail(c, 400, "表主已拥有权限")
		return
	}
	m := model.Member{TableID: tid(c), UserID: u.ID, Role: in.Role, Status: "approved"}
	if e := s.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "table_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"role": in.Role, "status": "approved"})}).Create(&m).Error; e != nil {
		fail(c, 500, "邀请失败")
		return
	}
	c.JSON(200, m)
}
func (s *Server) updateMember(c *gin.Context) {
	id, ok := idParam(c, "user")
	if !ok {
		return
	}
	if id == uid(c) {
		fail(c, 400, "不能修改表主权限")
		return
	}
	var in struct {
		Role   string `json:"role" binding:"required,oneof=viewer editor"`
		Status string `json:"status" binding:"required,oneof=approved rejected removed"`
	}
	if !bind(c, &in) {
		return
	}
	res := s.DB.Model(&model.Member{}).Where("table_id = ? AND user_id = ?", tid(c), id).Updates(map[string]any{"role": in.Role, "status": in.Status})
	if res.Error != nil {
		fail(c, 500, "保存失败")
		return
	}
	if res.RowsAffected == 0 {
		fail(c, 404, "成员不存在")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) data(c *gin.Context) {
	cats := []model.Cat{}
	litters := []model.Litter{}
	boxes := []model.Box{}
	rs := []model.Record{}
	weights := []model.Weight{}
	for _, dest := range []any{&cats, &litters, &boxes, &rs, &weights} {
		if e := s.DB.Where("table_id = ?", tid(c)).Order("id asc").Find(dest).Error; e != nil {
			fail(c, 500, "加载失败")
			return
		}
	}
	c.JSON(200, gin.H{"table": c.MustGet("table"), "role": c.GetString("role"), "join_status": c.GetString("join_status"), "cats": cats, "litters": litters, "boxes": boxes, "records": rs, "weights": weights, "analysis": analysis.AnalyzePools(rs, weights, boxes, litters, time.Now(), s.Config.TrainingTimezone)})
}
func (s *Server) resource(c *gin.Context, dest any) bool {
	id, ok := idParam(c, "id")
	if !ok {
		return false
	}
	if s.DB.Where("id = ? AND table_id = ?", id, tid(c)).First(dest).Error != nil {
		fail(c, 404, "记录不存在")
		return false
	}
	return true
}

type catInput struct {
	Name     string `json:"name" binding:"required,max=40"`
	Breed    string `json:"breed" binding:"max=60"`
	Sex      string `json:"sex" binding:"required,oneof=male female unknown"`
	Birthday string `json:"birthday"`
	Color    string `json:"color" binding:"oneof=sage peach lavender blue"`
	Neutered bool   `json:"neutered"`
	Archived bool   `json:"archived"`
	Notes    string `json:"notes" binding:"max=500"`
}

func (s *Server) saveCat(c *gin.Context) {
	var in catInput
	if !bind(c, &in) {
		return
	}
	if in.Birthday != "" {
		d, e := time.Parse("2006-01-02", in.Birthday)
		if e != nil || d.After(time.Now()) {
			fail(c, 400, "生日不正确")
			return
		}
	}
	var cat model.Cat
	if c.Param("id") != "" && !s.resource(c, &cat) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "请输入猫咪名称")
		return
	}
	cat.TableID = tid(c)
	cat.Name = strings.TrimSpace(in.Name)
	cat.Breed = in.Breed
	cat.Sex = in.Sex
	cat.Birthday = in.Birthday
	cat.Color = in.Color
	cat.Neutered = in.Neutered
	cat.Archived = in.Archived
	cat.Notes = in.Notes
	if e := s.DB.Save(&cat).Error; e != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, cat)
}

type litterInput struct {
	Name        string `json:"name" binding:"required,max=60"`
	Material    string `json:"material" binding:"required,oneof=tofu bentonite mixed silica paper other"`
	WaitMinutes int    `json:"wait_minutes" binding:"min=0,max=1440"`
	Notes       string `json:"notes" binding:"max=300"`
}

func (s *Server) saveLitter(c *gin.Context) {
	var in litterInput
	if !bind(c, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "请输入猫砂名称")
		return
	}
	l := model.Litter{Version: 1}
	if c.Param("id") != "" {
		if !s.resource(c, &l) {
			return
		}
		l.Version++
	}
	l.TableID = tid(c)
	l.Name = strings.TrimSpace(in.Name)
	l.Material = in.Material
	l.WaitMinutes = in.WaitMinutes
	l.Notes = in.Notes
	if e := s.DB.Save(&l).Error; e != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, l)
}

type boxInput struct {
	Name     string `json:"name" binding:"required,max=60"`
	Location string `json:"location" binding:"max=80"`
	LitterID uint   `json:"litter_id" binding:"required"`
	CatIDs   []uint `json:"cat_ids" binding:"required,min=1,max=100"`
	Archived bool   `json:"archived"`
}

func (s *Server) saveBox(c *gin.Context) {
	var in boxInput
	if !bind(c, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		fail(c, 400, "请输入猫砂盆名称")
		return
	}
	var l model.Litter
	if s.DB.First(&l, "id = ? AND table_id = ?", in.LitterID, tid(c)).Error != nil {
		fail(c, 400, "猫砂不属于此表格")
		return
	}
	seen := map[uint]bool{}
	for _, id := range in.CatIDs {
		if id == 0 || seen[id] {
			fail(c, 400, "使用猫名单不正确")
			return
		}
		seen[id] = true
	}
	var n int64
	if e := s.DB.Model(&model.Cat{}).Where("table_id = ? AND id IN ? AND archived = false", tid(c), in.CatIDs).Count(&n).Error; e != nil || int(n) != len(in.CatIDs) {
		fail(c, 400, "使用猫须属于此表格且未归档")
		return
	}
	var b model.Box
	if c.Param("id") != "" && !s.resource(c, &b) {
		return
	}
	b.TableID = tid(c)
	b.Name = strings.TrimSpace(in.Name)
	b.Location = in.Location
	b.LitterID = in.LitterID
	b.CatIDs = in.CatIDs
	b.Archived = in.Archived
	if e := s.DB.Save(&b).Error; e != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, b)
}

type recordInput struct {
	Kind                 string     `json:"kind" binding:"required,oneof=urine stool"`
	Mode                 string     `json:"mode" binding:"omitempty,oneof=baseline shared observed"`
	CatID                *uint      `json:"cat_id"`
	BoxID                uint       `json:"box_id" binding:"required"`
	OccurredAt           time.Time  `json:"occurred_at" binding:"required"`
	FormedAt             *time.Time `json:"formed_at"`
	LastCleanedAt        *time.Time `json:"last_cleaned_at"`
	LastCleaningComplete bool       `json:"last_cleaning_complete"`
	CollectionComplete   bool       `json:"collection_complete"`
	Integrity            string     `json:"integrity" binding:"omitempty,oneof=complete fragmented adhered mixed unknown"`
	EventID              string     `json:"event_id" binding:"max=64"`
	IdentityMethod       string     `json:"identity_method" binding:"omitempty,oneof=unknown isolated witnessed device manual"`
	BaselineVersion      int        `json:"baseline_version" binding:"min=0,max=10000"`
	LengthCM             *float64   `json:"length_cm" binding:"omitempty,gt=0,lte=50"`
	WidthCM              *float64   `json:"width_cm" binding:"omitempty,gt=0,lte=50"`
	ThicknessCM          *float64   `json:"thickness_cm" binding:"omitempty,gt=0,lte=50"`
	LitterDepthCM        *float64   `json:"litter_depth_cm" binding:"omitempty,gt=0,lte=30"`
	Count                int        `json:"count" binding:"min=0,max=100"`
	Grams                *float64   `json:"grams" binding:"omitempty,gt=0,lte=2000"`
	DiameterCM           *float64   `json:"diameter_cm" binding:"omitempty,gt=0,lte=50"`
	Looseness            *int       `json:"looseness" binding:"omitempty,min=1,max=5"`
	Moisture             *int       `json:"moisture" binding:"omitempty,min=1,max=5"`
	Shape                string     `json:"shape" binding:"omitempty,oneof=pellet formed soft paste watery"`
	Size                 *int       `json:"size" binding:"omitempty,min=1,max=5"`
	BodyWeightKG         *float64   `json:"body_weight_kg" binding:"omitempty,gt=0,lte=30"`
	Blood                bool       `json:"blood"`
	Straining            bool       `json:"straining"`
	NoOutput             bool       `json:"no_output"`
	Notes                string     `json:"notes" binding:"max=500"`
}

func contains(xs []uint, x uint) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func validDate(c *gin.Context, t time.Time) bool {
	if t.After(time.Now().Add(5*time.Minute)) || t.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		fail(c, 400, "时间须在2000年之后且不能在未来")
		return false
	}
	return true
}
func (s *Server) saveRecord(c *gin.Context) {
	var in recordInput
	if !bind(c, &in) || !validDate(c, in.OccurredAt) {
		return
	}
	var r model.Record
	editing := c.Param("id") != ""
	if editing && !s.resource(c, &r) {
		return
	}
	if !s.populateRecord(c, s.DB, &r, in, editing) {
		return
	}
	if e := s.DB.Save(&r).Error; e != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, r)
}
func (s *Server) populateRecord(c *gin.Context, db *gorm.DB, r *model.Record, in recordInput, editing bool) bool {
	if !validDate(c, in.OccurredAt) {
		return false
	}
	if in.Mode == "" {
		in.Mode = "observed"
	}
	if in.Integrity == "" {
		in.Integrity = "unknown"
	}
	if in.BaselineVersion == 0 {
		in.BaselineVersion = 1
	}
	if in.IdentityMethod == "" {
		in.IdentityMethod = "unknown"
		if in.Mode == "baseline" {
			in.IdentityMethod = "isolated"
		} else if in.CatID != nil {
			in.IdentityMethod = "manual"
		}
	}
	if in.FormedAt != nil && (!validDate(c, *in.FormedAt) || in.FormedAt.After(in.OccurredAt)) {
		if !c.IsAborted() {
			fail(c, 400, "形成时间不能晚于采集时间")
		}
		return false
	}
	if in.LastCleanedAt != nil && (!validDate(c, *in.LastCleanedAt) || !in.LastCleanedAt.Before(in.OccurredAt)) {
		if !c.IsAborted() {
			fail(c, 400, "上次清理时间须早于采集时间")
		}
		return false
	}
	if in.LastCleaningComplete && in.LastCleanedAt == nil {
		fail(c, 400, "确认上次彻底清理须填写清理时间")
		return false
	}
	if in.LastCleaningComplete && in.FormedAt != nil && in.FormedAt.Before(*in.LastCleanedAt) {
		fail(c, 400, "样本形成时间不能早于上次彻底清理")
		return false
	}
	if in.Mode == "baseline" && (in.IdentityMethod != "isolated" && in.IdentityMethod != "witnessed" && in.IdentityMethod != "device" || in.Integrity != "complete" && in.Integrity != "unknown") {
		fail(c, 400, "基准只接受独立确认身份的完整样本；碎裂、粘连和混合样本不能训练")
		return false
	}
	if in.Mode == "shared" && in.IdentityMethod != "unknown" {
		fail(c, 400, "共享样本身份须保持未知")
		return false
	}
	if in.NoOutput && (in.LengthCM != nil || in.WidthCM != nil || in.ThicknessCM != nil || in.FormedAt != nil) {
		fail(c, 400, "无产出记录不能填写样本尺寸或形成时间")
		return false
	}
	if in.EventID != "" {
		var previous []model.Record
		if db.Where("table_id = ? AND event_id = ? AND id <> ?", tid(c), strings.TrimSpace(in.EventID), r.ID).Find(&previous).Error != nil {
			fail(c, 500, "事件校验失败")
			return false
		}
		for _, p := range previous {
			if p.FormedAt != nil && in.FormedAt != nil && !p.FormedAt.Equal(*in.FormedAt) {
				fail(c, 400, "同一事件的形成时间须一致")
				return false
			}
			if p.Kind != in.Kind || p.CatID != nil && in.CatID != nil && *p.CatID != *in.CatID {
				fail(c, 400, "同一原始事件不能对应不同猫咪或不同排泄类型")
				return false
			}
		}
	}
	if in.Mode == "shared" && in.CatID != nil {
		fail(c, 400, "共享采集不需要指定猫咪，请清空归属")
		return false
	}
	if in.Mode == "baseline" && (in.CatID == nil || in.Count != 1 || in.NoOutput || in.Blood || in.Straining || in.Grams == nil) {
		fail(c, 400, "基线须为单猫隔离后的健康单块记录，指定猫咪、填写重量、个数为1")
		return false
	}
	if in.NoOutput {
		if in.Count != 0 || in.Grams != nil || in.DiameterCM != nil {
			fail(c, 400, "无产出时个数为0，不能填写重量和直径")
			return false
		}
	} else if in.Count < 1 {
		fail(c, 400, "有产出时个数至少为1")
		return false
	}
	if in.Kind == "urine" && (in.Moisture != nil || in.Size != nil || in.Shape != "") {
		fail(c, 400, "尿团记录不能包含大便维度")
		return false
	}
	if in.Kind == "stool" && (in.Looseness != nil || in.DiameterCM != nil) {
		fail(c, 400, "大便记录不能包含尿团维度")
		return false
	}
	if !in.NoOutput && in.Kind == "stool" && (in.Moisture == nil || in.Size == nil || in.Shape == "") {
		fail(c, 400, "请填写大便干湿、形态、大小")
		return false
	}
	var b model.Box
	if db.First(&b, "id = ? AND table_id = ?", in.BoxID, tid(c)).Error != nil || (!editing && b.Archived) {
		fail(c, 400, "猫砂盆不可用")
		return false
	}
	sameSnapshot := editing && r.BoxID == b.ID
	eligible := b.CatIDs
	if sameSnapshot {
		eligible = r.EligibleCatIDs
	}
	if in.CatID != nil {
		var cat model.Cat
		if db.First(&cat, "id = ? AND table_id = ?", *in.CatID, tid(c)).Error != nil || !contains(eligible, *in.CatID) {
			fail(c, 400, "此猫不在记录对应的猫砂盆使用名单中")
			return false
		}
	}
	if !sameSnapshot {
		var l model.Litter
		if db.First(&l, "id = ? AND table_id = ?", b.LitterID, tid(c)).Error != nil {
			fail(c, 400, "猫砂配置不可用")
			return false
		}
		r.LitterKey = fmt.Sprintf("%d:v%d", l.ID, l.Version)
		r.LitterName = l.Name
		r.WaitMinutes = l.WaitMinutes
		r.EligibleCatIDs = b.CatIDs
	}
	r.TableID = tid(c)
	if !editing {
		r.CreatorID = uid(c)
	}
	r.Kind = in.Kind
	r.Mode = in.Mode
	r.CatID = in.CatID
	r.BoxID = in.BoxID
	r.OccurredAt = in.OccurredAt
	r.FormedAt = in.FormedAt
	r.LastCleanedAt = in.LastCleanedAt
	r.LastCleaningComplete = in.LastCleaningComplete
	r.CollectionComplete = in.CollectionComplete
	r.Integrity = in.Integrity
	r.EventID = strings.TrimSpace(in.EventID)
	r.IdentityMethod = in.IdentityMethod
	r.BaselineVersion = in.BaselineVersion
	r.LengthCM = in.LengthCM
	r.WidthCM = in.WidthCM
	r.ThicknessCM = in.ThicknessCM
	r.LitterDepthCM = in.LitterDepthCM
	r.Count = in.Count
	r.Grams = in.Grams
	r.DiameterCM = in.DiameterCM
	r.Looseness = in.Looseness
	r.Moisture = in.Moisture
	r.Shape = in.Shape
	r.Size = in.Size
	r.BodyWeightKG = in.BodyWeightKG
	r.Blood = in.Blood
	r.Straining = in.Straining
	r.NoOutput = in.NoOutput
	r.Notes = in.Notes
	return true
}

func (s *Server) saveBatch(c *gin.Context) {
	var in struct {
		Items []recordInput `json:"items" binding:"required,min=1,max=30,dive"`
	}
	if !bind(c, &in) {
		return
	}
	items := []model.Record{}
	token := make([]byte, 12)
	if _, e := rand.Read(token); e != nil {
		fail(c, 500, "采集保存失败")
		return
	}
	batchID := hex.EncodeToString(token)
	invalid := errors.New("invalid batch")
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range in.Items {
			if item.Mode != "shared" || item.CatID != nil || item.Count != 1 || item.NoOutput || item.BoxID != in.Items[0].BoxID || !item.OccurredAt.Equal(in.Items[0].OccurredAt) {
				fail(c, 400, "一次共享采集须为同一盆、同一时间的逐块记录，不指定猫咪")
				return invalid
			}
			first := in.Items[0]
			equalTime := func(a, b *time.Time) bool { return a == nil && b == nil || a != nil && b != nil && a.Equal(*b) }
			if !equalTime(item.LastCleanedAt, first.LastCleanedAt) || item.LastCleaningComplete != first.LastCleaningComplete || item.CollectionComplete != first.CollectionComplete || (item.LitterDepthCM == nil) != (first.LitterDepthCM == nil) || item.LitterDepthCM != nil && *item.LitterDepthCM != *first.LitterDepthCM {
				fail(c, 400, "同一批次的清理时间、完整清理和铺砂厚度须一致")
				return invalid
			}
			for _, p := range items {
				if item.EventID != "" && p.EventID == strings.TrimSpace(item.EventID) && p.FormedAt != nil && item.FormedAt != nil && !p.FormedAt.Equal(*item.FormedAt) {
					fail(c, 400, "同一事件的形成时间须一致")
					return invalid
				}
				if item.EventID != "" && p.EventID == strings.TrimSpace(item.EventID) && p.Kind != item.Kind {
					fail(c, 400, "同一事件不能混用尿团和大便类型")
					return invalid
				}
			}
			r := model.Record{BatchID: batchID}
			if !s.populateRecord(c, tx, &r, item, false) {
				return invalid
			}
			items = append(items, r)
		}
		return tx.Create(&items).Error
	})
	if errors.Is(err, invalid) {
		return
	}
	if err != nil {
		fail(c, 500, "采集保存失败，未保存任何部分记录")
		return
	}
	c.JSON(201, gin.H{"batch_id": batchID, "records": items})
}
func (s *Server) deleteRecord(c *gin.Context) {
	var r model.Record
	if !s.resource(c, &r) {
		return
	}
	if s.DB.Delete(&r).Error != nil {
		fail(c, 500, "删除失败")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) saveWeight(c *gin.Context) {
	var in struct {
		CatID      uint      `json:"cat_id" binding:"required"`
		KG         float64   `json:"kg" binding:"required,gt=0,lte=30"`
		OccurredAt time.Time `json:"occurred_at" binding:"required"`
		Notes      string    `json:"notes" binding:"max=300"`
	}
	if !bind(c, &in) || !validDate(c, in.OccurredAt) {
		return
	}
	var cat model.Cat
	if s.DB.First(&cat, "id = ? AND table_id = ? AND archived = false", in.CatID, tid(c)).Error != nil {
		fail(c, 400, "猫咪不可用")
		return
	}
	w := model.Weight{TableID: tid(c), CatID: in.CatID, KG: in.KG, OccurredAt: in.OccurredAt, Notes: in.Notes, CreatorID: uid(c)}
	if s.DB.Create(&w).Error != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(201, w)
}
func (s *Server) deleteWeight(c *gin.Context) {
	var w model.Weight
	if !s.resource(c, &w) {
		return
	}
	if s.DB.Delete(&w).Error != nil {
		fail(c, 500, "删除失败")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
