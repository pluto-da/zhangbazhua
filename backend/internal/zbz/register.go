// Package zbz 是章八爪自有模块的后端入口。作者：长鱼茶汤茗。
// 上游代码只在 handler/api.go 里调用一次 Register，其余章八爪接口都在本目录内，
// 这样跟随上游合并时不会和作者的改动打架。
package zbz

import (
	"net/http"
	"os"
	"path/filepath"

	"infinite-canvas/backend/internal/app"

	"github.com/gin-gonic/gin"
)

// Register 挂载全部章八爪接口，前缀 /zbz。
func Register(api *gin.RouterGroup, dataDir string) {
	dir := filepath.Join(dataDir, "zbz")
	_ = os.MkdirAll(dir, 0o755)
	g := api.Group("/zbz")
	p := newPricing(dir)
	g.GET("/prices", p.handle)
	lv := newLive(dir, p)
	g.GET("/live", lv.handle)
	g.GET("/relays", lv.relays.list)
	g.POST("/relays", lv.relays.add)
	g.DELETE("/relays/:id", lv.relays.remove)
	g.GET("/relay-check", lv.relays.check)
	g.POST("/balance", handleBalance)
	l := newLedger(dir)
	g.GET("/ledger", l.list)
	g.POST("/ledger", l.add)
	g.DELETE("/ledger/:id", l.remove)
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": app.CodeOK, "data": data, "msg": "ok"})
}

func fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "data": nil, "msg": msg})
}
