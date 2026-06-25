package checker

import (
	"PanCheck/internal/model"
	apphttp "PanCheck/pkg/http"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// TianyiChecker 天翼云盘检测器
type TianyiChecker struct {
	*BaseChecker
}

// NewTianyiChecker 创建天翼云盘检测器
func NewTianyiChecker(concurrencyLimit int, timeout time.Duration) *TianyiChecker {
	return &TianyiChecker{
		BaseChecker: NewBaseChecker(model.PlatformTianyi, concurrencyLimit, timeout),
	}
}

// Check 检测链接是否有效
func (c *TianyiChecker) Check(link string) (*CheckResult, error) {
	// 应用频率限制
	c.ApplyRateLimit()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), c.GetTimeout())
	defer cancel()

	codeValue, accessCode, refererValue, err := extractCodeFromURL(link)
	if err != nil {
		return &CheckResult{
			Valid:         false,
			FailureReason: "链接格式无效: " + err.Error(),
			Duration:      time.Since(start).Milliseconds(),
		}, nil
	}

	response, err := telecomRequest(ctx, codeValue, accessCode, refererValue)
	duration := time.Since(start).Milliseconds()

	if err != nil {
		if apphttp.IsTimeoutError(err) {
			return &CheckResult{
				Valid:         false,
				FailureReason: "请求超时",
				Duration:      duration,
			}, nil
		}
		return &CheckResult{
			Valid:         false,
			FailureReason: "检测失败: " + err.Error(),
			Duration:      duration,
		}, nil
	}

	if response.ShareId > 0 {
		if response.NeedAccessCode == 1 && accessCode == "" {
			// 部分天翼短链本身就是完整分享串，接口仍会返回 needAccessCode=1，
			// 但这类链接不应被归类到需要额外提取码的 locked_links。
			if isSelfContainedTianyiShareLink(link) {
				return &CheckResult{
					Valid:         true,
					FailureReason: "",
					Duration:      duration,
				}, nil
			}
			return &CheckResult{
				Valid:               true,
				FailureReason:       "",
				Duration:            duration,
				IsPasswordProtected: true,
			}, nil
		}
		return &CheckResult{
			Valid:         true,
			FailureReason: "",
			Duration:      duration,
		}, nil
	}

	// shareId <= 0 或不存在，表示链接无效
	// 优先使用具体错误码生成失败原因
	failureReason := mapTelecomErrorMessage(response.ErrorCode, response.ResMessage)
	if failureReason == "" {
		failureReason = response.ResMessage
	}
	if failureReason == "" {
		failureReason = fmt.Sprintf("无法获取分享信息 (ShareId=%d)", response.ShareId)
	}
	return &CheckResult{
		Valid:         false,
		FailureReason: failureReason,
		Duration:      duration,
	}, nil
}

// TelecomResp 对应电信云盘API返回的数据结构
type TelecomResp struct {
	ResCode        int    `json:"res_code"`
	ResMessage     string `json:"res_message"`
	ErrorCode      string `json:"error_code"`
	FileName       string `json:"fileName"`
	NeedAccessCode int    `json:"needAccessCode"` // 是否需要访问码：1表示需要，0表示不需要
	ShareId        int64  `json:"shareId"`        // 分享ID，如果大于0表示链接有效
}

type telecomErrorResp struct {
	XMLName xml.Name `xml:"error"`
	Code    string   `xml:"code"`
	Message string   `xml:"message"`
}

// telecomRequest 发起请求
func telecomRequest(ctx context.Context, codeValue string, accessCode string, refererValue string) (*TelecomResp, error) {
	rand.Seed(time.Now().UnixNano())
	noCacheValue := rand.Float64()

	// 如果有访问码，需要将访问码包含在shareCode参数中
	// 格式：shareCode（访问码：accessCode）
	shareCodeParam := codeValue
	if accessCode != "" {
		shareCodeParam = fmt.Sprintf("%s（访问码：%s）", codeValue, accessCode)
	}

	shareCodeEncoded := url.QueryEscape(shareCodeParam)

	baseURL := "https://cloud.189.cn/api/open/share/getShareInfoByCodeV2.action"
	targetURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("解析基础URL失败: %v", err)
	}

	query := targetURL.Query()
	query.Set("noCache", fmt.Sprintf("%f", noCacheValue))
	query.Set("shareCode", shareCodeEncoded)
	targetURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL.String(), nil)

	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	apphttp.SetDefaultHeaders(req)
	req.Header.Set("priority", "u=1, i")
	req.Header.Set("referer", refererValue)
	req.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-fetch-dest", "empty")
	req.Header.Set("sec-fetch-mode", "cors")
	req.Header.Set("sec-fetch-site", "same-origin")
	req.Header.Set("sign-type", "1")

	httpClient := apphttp.GetClient()
	resp, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &apphttp.TimeoutError{Message: "请求超时"}
		}
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer apphttp.CloseResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	trimmedBody := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmedBody, "<") {
		var errorResp telecomErrorResp
		if err = xml.Unmarshal(body, &errorResp); err == nil && errorResp.Code != "" {
			return &TelecomResp{
				ResMessage: mapTelecomErrorMessage(errorResp.Code, errorResp.Message),
				ErrorCode:  errorResp.Code,
			}, nil
		}
	}

	var response TelecomResp
	if err = json.Unmarshal(body, &response); err != nil {
		log.Printf("[TianyiChecker] JSON解析失败，原始响应: %s", string(body))
		return nil, fmt.Errorf("解析JSON失败: %v", err)
	}

	// 如果 error_code 字段为空，尝试从响应体文本中扫描已知错误码
	bodyStr := string(body)
	if response.ErrorCode == "" {
		knownErrorCodes := []string{
			"ShareInfoNotFound",
			"ShareNotFound",
			"FileNotFound",
			"ShareExpiredError",
			"ShareAuditNotPass",
			"FolderNotFound",
		}
		for _, code := range knownErrorCodes {
			if strings.Contains(bodyStr, code) {
				response.ErrorCode = code
				break
			}
		}
	}

	return &response, nil
}

func mapTelecomErrorMessage(code string, fallback string) string {
	switch code {
	case "ShareInfoNotFound":
		return "分享信息不存在"
	case "ShareNotFound":
		return "分享链接不存在"
	case "FileNotFound":
		return "分享文件不存在"
	case "ShareExpiredError":
		return "分享链接已过期"
	case "ShareAuditNotPass":
		return "分享因审核未通过已失效"
	case "FolderNotFound":
		return "分享文件夹不存在"
	default:
		if fallback != "" {
			return fallback
		}
		return code
	}
}

// extractCodeFromURL 从URL中提取code参数和访问码
// 支持多种格式：
// 1. https://cloud.189.cn/web/share?code=xxx (从查询参数提取)
// 2. https://cloud.189.cn/t/xxx (从路径提取)
// 3. https://h5.cloud.189.cn/share.html#/t/xxx (从hash中提取)
// 4. https://cloud.189.cn/t/xxx（访问码：xxx） (从路径和文本中提取访问码)
// 返回值: codeValue, accessCode, refererValue, error
func extractCodeFromURL(urlStr string) (string, string, string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", "", fmt.Errorf("解析URL失败: %w", err)
	}

	var codeValue string
	var accessCode string

	queryParams := parsedURL.Query()
	codeValue = queryParams.Get("code")

	if codeValue == "" {
		path := parsedURL.Path
		if strings.HasPrefix(path, "/t/") {
			codeValue = strings.TrimPrefix(path, "/t/")
			if idx := strings.Index(codeValue, "/"); idx != -1 {
				codeValue = codeValue[:idx]
			}
			codeValue = trimCodeValue(codeValue)
		}
	}

	if codeValue == "" && parsedURL.Fragment != "" {
		fragment := parsedURL.Fragment
		if strings.HasPrefix(fragment, "/t/") {
			codeValue = strings.TrimPrefix(fragment, "/t/")
			if idx := strings.Index(codeValue, "/"); idx != -1 {
				codeValue = codeValue[:idx]
			}
			codeValue = trimCodeValue(codeValue)
		} else if strings.HasPrefix(fragment, "#/t/") {
			codeValue = strings.TrimPrefix(fragment, "#/t/")
			if idx := strings.Index(codeValue, "/"); idx != -1 {
				codeValue = codeValue[:idx]
			}
			codeValue = trimCodeValue(codeValue)
		}
	}

	if codeValue == "" {
		return "", "", "", fmt.Errorf("输入URL中未找到 'code' 参数（查询参数、路径或hash中）")
	}

	// 处理URL编码的code值
	if isURLEncoded(codeValue) && containsSpecialChars(codeValue) {
		decodedCode, err := url.QueryUnescape(codeValue)
		if err == nil && decodedCode != codeValue {
			codeValue = decodedCode
		}
	}

	// 从URL文本中提取访问码（访问码：xxx）
	// 支持格式：https://cloud.189.cn/t/xxx（访问码：xxx）
	accessCodePattern := regexp.MustCompile(`（访问码[：:]\s*([a-zA-Z0-9]+)）`)
	decodedURL, decodeErr := url.QueryUnescape(urlStr)
	if decodeErr != nil {
		decodedURL = urlStr
	}
	matches := accessCodePattern.FindStringSubmatch(decodedURL)
	if len(matches) >= 2 && matches[1] != "" {
		accessCode = matches[1]
	}
	if accessCode == "" {
		matches = accessCodePattern.FindStringSubmatch(urlStr)
		if len(matches) >= 2 && matches[1] != "" {
			accessCode = matches[1]
		}
	}

	parsedInputURL, err := url.Parse(urlStr)
	var refererValue string
	if err == nil {
		refererValue = parsedInputURL.String()
	} else {
		refererValue = urlStr
	}

	return codeValue, accessCode, refererValue, nil
}

// isURLEncoded 检查字符串是否包含URL编码特征
func isURLEncoded(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if isHex(s[i+1]) && isHex(s[i+2]) {
				return true
			}
		}
	}
	return false
}

// isHex 检查字符是否为十六进制数字
func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// containsSpecialChars 检查字符串是否包含需要URL编码的特殊字符
func containsSpecialChars(s string) bool {
	specialChars := " ()[]{}<>!@#$%^&*+=|\\:;\"',?/~"
	for _, char := range s {
		if strings.ContainsRune(specialChars, char) {
			return true
		}
	}
	return false
}

func trimCodeValue(code string) string {
	end := len(code)
	for i, r := range code {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			end = i
			break
		}
	}
	return code[:end]
}

func isSelfContainedTianyiShareLink(urlStr string) bool {
	if strings.Contains(urlStr, "访问码") {
		return false
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsedURL.Host)
	switch host {
	case "cloud.189.cn", "h5.cloud.189.cn":
	default:
		return false
	}

	if strings.HasPrefix(parsedURL.Path, "/t/") {
		return trimCodeValue(strings.TrimPrefix(parsedURL.Path, "/t/")) != ""
	}

	fragment := strings.TrimPrefix(parsedURL.Fragment, "#")
	if strings.HasPrefix(fragment, "/t/") {
		return trimCodeValue(strings.TrimPrefix(fragment, "/t/")) != ""
	}

	return false
}
