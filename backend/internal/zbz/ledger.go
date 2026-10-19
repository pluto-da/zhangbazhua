// 章八爪模块 · 充值中心本地账本 · 作者 长鱼茶汤茗
// 只记金额、平台、时间、备注；不保存任何支付信息。
package zbz

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type entry struct {
	ID       string  `json:"id"`
	Platform string  `json:"platform"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Note     string  `json:"note"`
	At       string  `json:"at"`
}

type ledger struct {
	mu   sync.Mutex
	path string
}

func newLedger(dir string) *ledger { return &ledger{path: filepath.Join(dir, "ledger.json")} }

func (l *ledger) load() []entry {
	var out []entry
	if b, err := os.ReadFile(l.path); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (l *ledger) save(list []entry) error {
	b, _ := json.MarshalIndent(list, "", "  ")
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

func (l *ledger) list(c *gin.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	list := l.load()
	if list == nil {
		list = []entry{}
	}
	ok(c, gin.H{"entries": list})
}

func (l *ledger) add(c *gin.Context) {
	var in entry
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Platform) == "" || in.Amount <= 0 {
		fail(c, http.StatusBadRequest, "请填写平台和大于 0 的金额")
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	in.ID = strconv.FormatInt(time.Now().UnixNano(), 36)
	if in.At == "" {
		in.At = time.Now().Format(time.RFC3339)
	}
	if in.Currency == "" {
		in.Currency = "CNY"
	}
	list := append(l.load(), in)
	if err := l.save(list); err != nil {
		fail(c, http.StatusInternalServerError, "账本保存失败")
		return
	}
	ok(c, in)
}

func (l *ledger) remove(c *gin.Context) {
	id := c.Param("id")
	l.mu.Lock()
	defer l.mu.Unlock()
	list := l.load()
	out := list[:0]
	for _, e := range list {
		if e.ID != id {
			out = append(out, e)
		}
	}
	if err := l.save(out); err != nil {
		fail(c, http.StatusInternalServerError, "账本保存失败")
		return
	}
	ok(c, gin.H{"deleted": id})
}

func ioCopyLimit(w io.Writer, resp *http.Response) (int64, error) {
	return io.Copy(w, io.LimitReader(resp.Body, 1<<20))
}
