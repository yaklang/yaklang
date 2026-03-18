package vulinbox

import (
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

func (s *VulinServer) registerHotPatchPipelineRoute() {
	store := newHotPatchPipelineSessionStore()
	router := s.router.PathPrefix("/api/pipeline").Name("全局热加载 Pipeline 教学").Subrouter()

	addRouteWithVulInfo(router, &VulInfo{
		Path:  "/console",
		Title: "",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			hotPatchPipelineRedirectConsole(writer, request)
		},
	})

	addRouteWithVulInfo(router, &VulInfo{
		Path:  "/docs",
		Title: "全局热加载 Pipeline",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			hotPatchPipelineRenderDocs(writer, request)
		},
	})

	addRouteWithVulInfo(router, &VulInfo{
		Path:  "/bootstrap",
		Title: "",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			ticket, err := store.issueTicket()
			if err != nil {
				log.Errorf("issue pipeline ticket failed: %v", err)
				hotPatchPipelineWritePlainJSON(writer, http.StatusInternalServerError, map[string]any{
					"message": "issue ticket failed",
				})
				return
			}
			hotPatchPipelineWritePlainJSON(writer, http.StatusOK, ticket)
		},
	})

	addRouteWithVulInfo(router, &VulInfo{
		Path:         "/orders/search",
		Title:        "",
		RiskDetected: true,
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodPost {
				hotPatchPipelineWritePlainJSON(writer, http.StatusMethodNotAllowed, map[string]any{
					"message": "use POST with application/json",
				})
				return
			}

			body, err := io.ReadAll(io.LimitReader(request.Body, 4096))
			if err != nil {
				hotPatchPipelineWritePlainJSON(writer, http.StatusBadRequest, map[string]any{
					"message": "read request body failed",
				})
				return
			}

			sessionID := request.Header.Get("X-Pipeline-Session")
			timestamp := request.Header.Get("X-Pipeline-Timestamp")
			signatureHex := request.Header.Get("X-Pipeline-Signature")
			if sessionID == "" || timestamp == "" || signatureHex == "" {
				hotPatchPipelineWritePlainJSON(writer, http.StatusUnauthorized, map[string]any{
					"message": "missing X-Pipeline-Session / X-Pipeline-Timestamp / X-Pipeline-Signature",
				})
				return
			}

			session, ok := store.get(sessionID)
			if !ok {
				hotPatchPipelineWritePlainJSON(writer, http.StatusUnauthorized, map[string]any{
					"message": "session expired or not found",
				})
				return
			}

			timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
			if err != nil {
				hotPatchPipelineWritePlainJSON(writer, http.StatusBadRequest, map[string]any{
					"message": "invalid X-Pipeline-Timestamp",
				})
				return
			}
			if hotPatchPipelineTimeDelta(time.Unix(timestampInt, 0), time.Now()) > hotPatchPipelineClockSkew {
				hotPatchPipelineWritePlainJSON(writer, http.StatusUnauthorized, map[string]any{
					"message": "timestamp expired",
				})
				return
			}

			clientSignature, err := hex.DecodeString(signatureHex)
			if err != nil {
				hotPatchPipelineWritePlainJSON(writer, http.StatusBadRequest, map[string]any{
					"message": "invalid X-Pipeline-Signature",
				})
				return
			}

			expectedSignature := hotPatchPipelineSignature(
				request.Method,
				request.URL.Path,
				timestamp,
				session.Key,
			)
			if !hmac.Equal(expectedSignature, clientSignature) {
				hotPatchPipelineWritePlainJSON(writer, http.StatusForbidden, map[string]any{
					"message": "signature verify failed",
				})
				return
			}

			var req struct {
				Keyword string `json:"keyword"`
				Status  string `json:"status"`
				Page    int    `json:"page"`
				Size    int    `json:"size"`
			}
			if err = json.Unmarshal(body, &req); err != nil {
				hotPatchPipelineWriteEncryptedJSON(writer, http.StatusBadRequest, session, map[string]any{
					"message": "invalid json body",
				})
				return
			}

			page, size := hotPatchPipelinePaging(req.Page, req.Size)
			status := req.Status
			if status == "" {
				status = hotPatchPipelineDefaultStatus
			}

			query := fmt.Sprintf(`
select
	user_orders.id as order_id,
	vulin_users.username as username,
	user_orders.ProductName as product_name,
	user_orders.Quantity as quantity,
	user_orders.TotalPrice as total_price,
	user_orders.DeliveryStatus as delivery_status
from user_orders
join vulin_users on user_orders.UserID = vulin_users.id
where user_orders.deleted_at is null
  and vulin_users.deleted_at is null
  and user_orders.DeliveryStatus = '%s'
  and user_orders.ProductName like '%%%s%%'
order by user_orders.id desc
limit %d offset %d;
`, status, req.Keyword, size, (page-1)*size)

			rows, err := s.database.UnsafeSqlQuery(query)
			if err != nil {
				hotPatchPipelineWriteEncryptedJSON(writer, http.StatusInternalServerError, session, map[string]any{
					"message": "query failed",
					"error":   err.Error(),
				})
				return
			}

			hotPatchPipelineWriteEncryptedJSON(writer, http.StatusOK, session, map[string]any{
				"page":      page,
				"size":      size,
				"row_count": len(rows),
				"rows":      hotPatchPipelineNormalizeRows(rows),
				"scene":     "global-before-sign -> module-before-mutate -> global-after-decrypt -> module-after-judge",
			})
		},
	})
}
