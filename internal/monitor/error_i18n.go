package monitor

import (
	"regexp"
	"strings"
)

var localizedExactMessages = map[string]string{
	"Invalid JSON request body":                              "请求体 JSON 无效",
	"URL must not be empty":                                  "链接不能为空",
	"URL is invalid":                                         "链接格式无效",
	"Only http/https URLs are supported":                     "仅支持 http/https 链接",
	"URL is missing a host name":                             "链接缺少主机名",
	"Failed to open external URL":                            "打开链接失败",
	"Hot service is unavailable":                             "热门服务不可用",
	"Symbol is required":                                     "股票代码不能为空",
	"Quantity must not be negative":                          "持仓数量不能小于 0",
	"Price must not be negative":                             "价格不能小于 0",
	"Alert name is required":                                 "提醒名称不能为空",
	"Alert must reference an item":                           "提醒必须关联一个标的",
	"Alert condition is invalid":                             "提醒条件无效",
	"Alert threshold must be greater than 0":                 "提醒阈值必须大于 0",
	"Refresh interval must be at least 10 seconds":           "刷新间隔不能低于 10 秒",
	"China quote source is invalid":                          "A股行情来源无效",
	"Hong Kong quote source is invalid":                      "港股行情来源无效",
	"US quote source is invalid":                             "美股行情来源无效",
	"Alpha Vantage API key is required":                      "使用 Alpha Vantage 时必须填写 API key",
	"Twelve Data API key is required":                        "使用 Twelve Data 时必须填写 API key",
	"Finnhub API key is required":                            "使用 Finnhub 时必须填写 API key",
	"Polygon API key is required":                            "使用 Polygon 时必须填写 API key",
	"Font preset must be one of: system / reading / compact": "字体预设仅支持 system / reading / compact",
	"Theme mode must be one of: system / light / dark":       "外观模式仅支持 system / light / dark",
	"Color theme must be one of: blue / graphite / forest / sunset / rose / violet / amber": "界面配色仅支持 blue / graphite / forest / sunset / rose / violet / amber",
	"Amount display must be one of: full / compact":                                         "金额展示仅支持 full / compact",
	"Currency display must be one of: symbol / code":                                        "币种展示仅支持 symbol / code",
	"Price color scheme must be one of: cn / intl":                                          "涨跌配色仅支持 cn / intl",
	"Locale must be one of: system / zh-CN / en-US":                                         "语言区域仅支持 system / zh-CN / en-US",
	"Dashboard currency must be one of: CNY / HKD / USD":                                    "展示货币仅支持 CNY / HKD / USD",
	"US hot source must be one of: eastmoney / yahoo":                                       "热门美股来源仅支持 eastmoney / yahoo",
	"History provider is not configured":                                                    "历史行情 provider 未配置",
	"No quote symbols are available in the hot fallback pool":                               "热门备援列表没有可请求的行情代码",
	"No results returned":                                                                   "返回空结果",
	"History response is missing price data":                                                "历史行情缺少价格数据",
	"History response contains no valid price points":                                       "历史行情缺少有效价格点",
	"History interval must be one of: 1h / 1d / 1w / 1mo / 1y / 3y / all":                   "图表范围仅支持 1h / 1d / 1w / 1mo / 1y / 3y / all",
	"Yahoo quote response is empty":                                                         "Yahoo 行情为空",
	"Yahoo quote response contains no valid price points":                                   "Yahoo 行情缺少有效价格点",
	"Polygon quote response is empty":                                                       "Polygon 行情为空",
	"EastMoney history response is empty":                                                   "东方财富历史行情无数据",
	"EastMoney history contains no valid price points":                                      "东方财富历史行情无有效价格点",
	"EastMoney history contains no valid price points after trimming":                       "东方财富历史行情裁剪后无有效价格点",
	"FX payload is invalid":                                                                 "汇率数据格式异常",
	"Hot fallback quote response is empty":                                                  "热门备援行情无数据",
}

var localizedPrefixMessages = []struct {
	prefix    string
	zhPrefix  string
	recursive bool
}{
	{prefix: "API route not found: ", zhPrefix: "接口不存在: "},
	{prefix: "Unrecognized symbol: ", zhPrefix: "无法识别股票代码: "},
	{prefix: "Cannot infer market for numeric symbol: ", zhPrefix: "无法识别数字代码归属市场: "},
	{prefix: "A-share symbol must be 6 digits: ", zhPrefix: "A 股代码应为 6 位: "},
	{prefix: "Beijing Exchange symbol must be 6 digits: ", zhPrefix: "北交所代码应为 6 位: "},
	{prefix: "Hong Kong symbol must be numeric: ", zhPrefix: "港股代码必须为数字: "},
	{prefix: "Hong Kong symbol length is invalid: ", zhPrefix: "港股代码长度异常: "},
	{prefix: "US symbol is invalid: ", zhPrefix: "美股代码格式无效: "},
	{prefix: "A-share / fund symbol must be 6 digits: ", zhPrefix: "A股/基金代码应为 6 位: "},
	{prefix: "Cannot recognize A-share / ETF symbol: ", zhPrefix: "无法识别A股/ETF代码: "},
	{prefix: "Item already exists in the list: ", zhPrefix: "该标的已在列表中: "},
	{prefix: "Item not found: ", zhPrefix: "标的不存在: "},
	{prefix: "Alert item not found: ", zhPrefix: "提醒关联的标的不存在: "},
	{prefix: "Alert not found: ", zhPrefix: "提醒不存在: "},
	{prefix: "Hot category is unsupported: ", zhPrefix: "不支持的热门分类: "},
	{prefix: "EastMoney hot category is unsupported: ", zhPrefix: "不支持的东方财富热门分类: "},
	{prefix: "EastMoney hot request failed: status ", zhPrefix: "东方财富热门请求失败: 状态码 "},
	{prefix: "EastMoney hot response returned rc=", zhPrefix: "东方财富热门返回 rc="},
	{prefix: "No available hot pool for category: ", zhPrefix: "热门分类暂无可用数据池: "},
	{prefix: "Yahoo does not support item: ", zhPrefix: "Yahoo 不支持该标的: "},
	{prefix: "Finnhub does not support item: ", zhPrefix: "Finnhub 不支持该标的: "},
	{prefix: "Polygon does not support item: ", zhPrefix: "Polygon 不支持该标的: "},
	{prefix: "Yahoo quote request failed: ", zhPrefix: "Yahoo 行情请求失败: ", recursive: true},
	{prefix: "Alpha Vantage quote request failed: ", zhPrefix: "Alpha Vantage 行情请求失败: ", recursive: true},
	{prefix: "Twelve Data quote request failed: ", zhPrefix: "Twelve Data 行情请求失败: ", recursive: true},
	{prefix: "Finnhub quote request failed: ", zhPrefix: "Finnhub 行情请求失败: ", recursive: true},
	{prefix: "Polygon quote request failed: ", zhPrefix: "Polygon 行情请求失败: ", recursive: true},
	{prefix: "EastMoney quote request failed: status ", zhPrefix: "东方财富行情请求失败: 状态码 "},
	{prefix: "EastMoney quote response returned rc=", zhPrefix: "东方财富行情返回 rc="},
	{prefix: "Did not receive EastMoney quote for ", zhPrefix: "未收到 ", recursive: false},
	{prefix: "A-share / ETF symbol format is invalid: ", zhPrefix: "A股/ETF代码格式错误: "},
	{prefix: "Realtime quotes are not supported for Beijing Exchange symbols in EastMoney: ", zhPrefix: "东方财富暂不支持北交所实时行情: "},
	{prefix: "Hong Kong symbol format is invalid: ", zhPrefix: "港股代码格式错误: "},
	{prefix: "US symbol format is invalid: ", zhPrefix: "美股代码格式错误: "},
	{prefix: "Market type is unsupported: ", zhPrefix: "不支持的市场类型: "},
	{prefix: "History request failed: ", zhPrefix: "历史行情请求失败: ", recursive: true},
	{prefix: "Alpha Vantage history request failed: ", zhPrefix: "Alpha Vantage 历史行情请求失败: ", recursive: true},
	{prefix: "Twelve Data history request failed: ", zhPrefix: "Twelve Data 历史行情请求失败: ", recursive: true},
	{prefix: "Finnhub history request failed: ", zhPrefix: "Finnhub 历史行情请求失败: ", recursive: true},
	{prefix: "Polygon history request failed: ", zhPrefix: "Polygon 历史行情请求失败: ", recursive: true},
	{prefix: "Yahoo does not support market: ", zhPrefix: "Yahoo 不支持该市场: "},
	{prefix: "Finnhub does not support market: ", zhPrefix: "Finnhub 不支持该市场: "},
	{prefix: "Polygon does not support market: ", zhPrefix: "Polygon 不支持该市场: "},
	{prefix: "EastMoney history request failed: ", zhPrefix: "东方财富历史行情请求失败: ", recursive: true},
	{prefix: "EastMoney history decode failed: ", zhPrefix: "东方财富历史行情解析失败: ", recursive: true},
	{prefix: "EastMoney history response returned rc=", zhPrefix: "东方财富历史行情返回 rc="},
	{prefix: "EastMoney does not support history interval: ", zhPrefix: "东方财富不支持该历史范围: "},
	{prefix: "Hot fallback quote request failed: status ", zhPrefix: "热门备援行情请求失败: 状态码 "},
	{prefix: "Hot fallback quote response returned rc=", zhPrefix: "热门备援行情返回 rc="},
	{prefix: "Failed to create FX request: ", zhPrefix: "创建汇率请求失败: ", recursive: true},
	{prefix: "FX service is unreachable: ", zhPrefix: "汇率服务不可达: ", recursive: true},
	{prefix: "Failed to read FX response: ", zhPrefix: "读取汇率响应失败: ", recursive: true},
	{prefix: "FX service returned ", zhPrefix: "汇率服务返回 ", recursive: false},
	{prefix: "Failed to decode FX data: ", zhPrefix: "解析汇率数据失败: ", recursive: true},
	{prefix: "status ", zhPrefix: "状态码 "},
	{prefix: "unexpected status ", zhPrefix: "状态码异常 "},
}

var (
	didNotReceiveEastMoneyQuotePattern = regexp.MustCompile(`^Did not receive EastMoney quote for (.+) \((.+)\)$`)
	eastMoneyResolveItemPattern        = regexp.MustCompile(`^EastMoney history failed to resolve item (.+?): (.+)$`)
	eastMoneyResolveSecIDPattern       = regexp.MustCompile(`^EastMoney history failed to resolve secid: (.+)$`)
	providerProblemPattern             = regexp.MustCompile(`^(EastMoney|Yahoo Finance): (.+)$`)
)

// NormalizeLocale collapses system and language tags to the app's supported locales.
func NormalizeLocale(locale string) string {
	normalized := strings.ToLower(strings.TrimSpace(locale))
	if strings.HasPrefix(normalized, "zh") {
		return "zh-CN"
	}
	return "en-US"
}

// LocalizeErrorMessage converts the canonical backend error text into the requested locale.
func LocalizeErrorMessage(locale, message string) string {
	clean := strings.TrimSpace(message)
	if clean == "" {
		return ""
	}

	if NormalizeLocale(locale) != "zh-CN" {
		return normalizeProblemSeparators(clean)
	}

	parts := splitProblemMessages(clean)
	localized := make([]string, 0, len(parts))
	for _, part := range parts {
		if candidate := strings.TrimSpace(part); candidate != "" {
			localized = append(localized, localizeSingleError(candidate))
		}
	}
	if len(localized) == 0 {
		return clean
	}
	return strings.Join(localized, "；")
}

func splitProblemMessages(message string) []string {
	normalized := strings.ReplaceAll(message, "；", ";")
	return strings.Split(normalized, ";")
}

func normalizeProblemSeparators(message string) string {
	parts := splitProblemMessages(message)
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if candidate := strings.TrimSpace(part); candidate != "" {
			normalized = append(normalized, candidate)
		}
	}
	if len(normalized) == 0 {
		return strings.TrimSpace(message)
	}
	return strings.Join(normalized, "; ")
}

func localizeSingleError(message string) string {
	if message == "" {
		return ""
	}

	if translated, ok := localizedExactMessages[message]; ok {
		return translated
	}

	if parts := didNotReceiveEastMoneyQuotePattern.FindStringSubmatch(message); parts != nil {
		return "未收到 " + parts[1] + " 的东方财富行情 (" + parts[2] + ")"
	}
	if parts := eastMoneyResolveItemPattern.FindStringSubmatch(message); parts != nil {
		return "东方财富历史行情: 无法解析标的 " + parts[1] + ": " + localizeSingleError(parts[2])
	}
	if parts := eastMoneyResolveSecIDPattern.FindStringSubmatch(message); parts != nil {
		return "东方财富历史行情: 无法解析 secid: " + localizeSingleError(parts[1])
	}
	if parts := providerProblemPattern.FindStringSubmatch(message); parts != nil {
		name := parts[1]
		if name == "EastMoney" {
			name = "东方财富"
		}
		return name + ": " + localizeSingleError(parts[2])
	}

	for _, entry := range localizedPrefixMessages {
		if !strings.HasPrefix(message, entry.prefix) {
			continue
		}
		suffix := strings.TrimPrefix(message, entry.prefix)
		if entry.recursive {
			suffix = localizeSingleError(suffix)
		}
		return entry.zhPrefix + suffix
	}

	return message
}
