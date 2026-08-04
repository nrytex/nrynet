package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type TransportManager interface {
	Status(context.Context) (TransportStatus, error)
	RequestCertificate(context.Context, CertificateRequest) (TransportStatus, error)
	SetTLSEnabled(context.Context, bool) (TransportStatus, error)
	SetPlainEnabled(context.Context, bool) (TransportStatus, error)
	SetAutoSubdomain(context.Context, AutoSubdomainRequest) (TransportStatus, error)
}

type TransportStatus struct {
	Plain              TransportEndpoint      `json:"plain"`
	CompatibilityPlain TransportEndpoint      `json:"compatibility_plain"`
	TLS                TransportEndpoint      `json:"tls"`
	Certbot            CertbotStatus          `json:"certbot"`
	Certificate        *CertificateStatus     `json:"certificate,omitempty"`
	AutoSubdomain      TransportAutoSubdomain `json:"auto_subdomain"`
	Capabilities       TransportCapabilities  `json:"capabilities,omitempty"`
}

type TransportEndpoint struct {
	Enabled      bool   `json:"enabled"`
	Listen       string `json:"listen,omitempty"`
	DataListen   string `json:"data_listen,omitempty"`
	ControlURL   string `json:"control_url,omitempty"`
	WebSocketURL string `json:"websocket_url,omitempty"`
	DataAddress  string `json:"data_address,omitempty"`
}

type CertbotStatus struct {
	Available bool   `json:"available"`
	Message   string `json:"message,omitempty"`
	Version   string `json:"version,omitempty"`
}

type CertificateStatus struct {
	Domain   string `json:"domain,omitempty"`
	Email    string `json:"email,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
	NotAfter string `json:"not_after,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
	Details  string `json:"details,omitempty"`
}

type TransportCapabilities struct {
	CertbotAvailable bool   `json:"certbot_available"`
	CertbotMessage   string `json:"certbot_message,omitempty"`
	HotReload        bool   `json:"hot_reload"`
}

type TransportAutoSubdomain struct {
	Enabled       bool   `json:"enabled"`
	BaseDomain    string `json:"base_domain,omitempty"`
	SuffixExample string `json:"suffix_example,omitempty"`
}

type CertificateRequest struct {
	Domain string `json:"domain"`
	Email  string `json:"email"`
}

type AutoSubdomainRequest struct {
	Enabled    *bool  `json:"enabled"`
	BaseDomain string `json:"base_domain"`
}

type enabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type transportHandler struct {
	manager TransportManager
}

func newTransportHandler(manager TransportManager) transportHandler {
	if manager == nil {
		manager = unavailableTransport{}
	}
	return transportHandler{manager: manager}
}

func (h transportHandler) get(c *gin.Context) {
	status, err := h.manager.Status(c.Request.Context())
	respondTransport(c, http.StatusOK, status, err)
}

func (h transportHandler) requestCertificate(c *gin.Context) {
	var request CertificateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respondError(c, http.StatusBadRequest, "\u8bf7\u8f93\u5165\u57df\u540d\u548c\u90ae\u7bb1")
		return
	}
	status, err := h.manager.RequestCertificate(c.Request.Context(), request)
	respondTransport(c, http.StatusAccepted, status, err)
}

func (h transportHandler) setTLS(c *gin.Context) {
	h.setEnabled(c, h.manager.SetTLSEnabled)
}

func (h transportHandler) setPlain(c *gin.Context) {
	h.setEnabled(c, h.manager.SetPlainEnabled)
}

func (h transportHandler) setAutoSubdomain(c *gin.Context) {
	var request AutoSubdomainRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		respondError(c, http.StatusBadRequest, "请选择开启或关闭")
		return
	}
	status, err := h.manager.SetAutoSubdomain(c.Request.Context(), request)
	respondTransport(c, http.StatusOK, status, err)
}

func (h transportHandler) setEnabled(c *gin.Context, update func(context.Context, bool) (TransportStatus, error)) {
	var request enabledRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		respondError(c, http.StatusBadRequest, "\u8bf7\u9009\u62e9\u5f00\u542f\u6216\u5173\u95ed")
		return
	}
	status, err := update(c.Request.Context(), *request.Enabled)
	respondTransport(c, http.StatusOK, status, err)
}

func respondTransport(c *gin.Context, success int, status TransportStatus, err error) {
	if err != nil {
		respondError(c, transportErrorStatus(err), friendlyTransportError(err))
		return
	}
	c.JSON(success, status)
}

func transportErrorStatus(err error) int {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "certbot") && (strings.Contains(message, "not found") || strings.Contains(message, "not available")) {
		return http.StatusServiceUnavailable
	}
	if strings.Contains(message, "not available") || strings.Contains(message, "\u4e0d\u53ef\u7528") {
		return http.StatusServiceUnavailable
	}
	if strings.Contains(message, "invalid") || strings.Contains(message, "required") || strings.Contains(message, "\u8bf7\u8f93\u5165") {
		return http.StatusBadRequest
	}
	if strings.Contains(message, "certificate") || strings.Contains(message, "tls") {
		return http.StatusConflict
	}
	if strings.Contains(message, "pending") || strings.Contains(message, "already") {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func friendlyTransportError(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "pending") || strings.Contains(message, "already"):
		return "已有证书任务正在处理，请稍后再试"
	case strings.Contains(message, "certbot") && (strings.Contains(message, "not found") || strings.Contains(message, "not available")):
		return "\u670d\u52a1\u5668\u672a\u5b89\u88c5\u6216\u672a\u542f\u7528 Certbot\uff0c\u65e0\u6cd5\u81ea\u52a8\u7533\u8bf7\u8bc1\u4e66"
	case strings.Contains(message, "domain"):
		return "\u57df\u540d\u65e0\u6548\uff0c\u8bf7\u586b\u5199\u6b63\u786e\u7684\u516c\u7f51\u57df\u540d"
	case strings.Contains(message, "email"):
		return "\u90ae\u7bb1\u65e0\u6548\uff0c\u8bf7\u586b\u5199\u7528\u4e8e\u8bc1\u4e66\u901a\u77e5\u7684\u90ae\u7bb1"
	case strings.Contains(message, "certificate") || strings.Contains(message, "tls"):
		return "\u8bc1\u4e66\u4e0d\u53ef\u7528\uff0c\u8bf7\u5148\u7ed1\u5b9a\u57df\u540d\u5e76\u7533\u8bf7\u8bc1\u4e66"
	case strings.Contains(message, "plain") || strings.Contains(message, "\u660e\u6587"):
		return "\u660e\u6587\u8bbf\u95ee\u914d\u7f6e\u65e0\u6548\uff0c\u8bf7\u68c0\u67e5 HTTP/WS \u548c\u6570\u636e\u901a\u9053\u5730\u5740"
	default:
		return err.Error()
	}
}

type unavailableTransport struct{}

func (unavailableTransport) Status(context.Context) (TransportStatus, error) {
	return TransportStatus{}, errTransportUnavailable()
}

func (unavailableTransport) RequestCertificate(context.Context, CertificateRequest) (TransportStatus, error) {
	return TransportStatus{}, errTransportUnavailable()
}

func (unavailableTransport) SetTLSEnabled(context.Context, bool) (TransportStatus, error) {
	return TransportStatus{}, errTransportUnavailable()
}

func (unavailableTransport) SetPlainEnabled(context.Context, bool) (TransportStatus, error) {
	return TransportStatus{}, errTransportUnavailable()
}

func (unavailableTransport) SetAutoSubdomain(context.Context, AutoSubdomainRequest) (TransportStatus, error) {
	return TransportStatus{}, errTransportUnavailable()
}

func errTransportUnavailable() error {
	return transportUnavailableError{}
}

type transportUnavailableError struct{}

func (transportUnavailableError) Error() string { return "transport manager not available" }
