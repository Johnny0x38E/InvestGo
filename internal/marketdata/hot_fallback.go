package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"investgo/internal/datasource"
	"investgo/internal/monitor"
)

type hotSeed struct {
	Symbol   string
	Name     string
	Market   string
	Currency string
}

// fetchPoolQuotes requests real-time quotes in batch for the predefined hot category constituent pool and returns them in a unified format.
func (s *HotService) fetchPoolQuotes(ctx context.Context, seeds []hotSeed) ([]monitor.HotItem, error) {
	secids := make([]string, 0, len(seeds)*2)
	indexBySecID := make(map[string]hotSeed, len(seeds)*2)
	for _, seed := range seeds {
		ids, err := resolveAllPoolSecIDs(seed)
		if err != nil {
			continue
		}
		for _, secid := range ids {
			secids = append(secids, secid)
			indexBySecID[secid] = seed
		}
	}

	if len(secids) == 0 {
		return nil, fmt.Errorf("No quote symbols are available in the hot fallback pool")
	}

	params := url.Values{}
	params.Set("fltt", "2")
	params.Set("invt", "2")
	params.Set("np", "1")
	params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
	params.Set("fields", "f2,f3,f4,f5,f12,f13,f14,f20")
	params.Set("secids", strings.Join(secids, ","))

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, datasource.URLWithQuery(datasource.EastMoneyQuoteAPI, params), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Referer", datasource.EastMoneyWebReferer)
	request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hot fallback quote request failed: status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var parsed eastMoneyHotResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, err
	}
	if parsed.RC != 0 {
		return nil, fmt.Errorf("Hot fallback quote response returned rc=%d", parsed.RC)
	}

	items := make([]monitor.HotItem, 0, len(parsed.Data.Diff))
	seen := make(map[string]struct{}, len(parsed.Data.Diff))
	for _, item := range parsed.Data.Diff {
		secid := fmt.Sprintf("%d.%s", item.MarketID, normaliseEastMoneyCode(item.Code, item.MarketID))
		seed, ok := indexBySecID[secid]
		if !ok {
			continue
		}

		key := seed.Market + "|" + seed.Symbol
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		items = append(items, monitor.HotItem{
			Symbol:        seed.Symbol,
			Name:          firstNonEmpty(item.Name, seed.Name),
			Market:        seed.Market,
			Currency:      seed.Currency,
			CurrentPrice:  float64(item.CurrentPrice),
			Change:        float64(item.Change),
			ChangePercent: float64(item.ChangePercent),
			Volume:        float64(item.Volume),
			MarketCap:     float64(item.MarketCap),
			QuoteSource:   "EastMoney",
			UpdatedAt:     time.Now(),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("Hot fallback quote response is empty")
	}

	return items, nil
}

// resolveAllPoolSecIDs returns all possible secids for the seed instrument.
// For US stocks, it returns the 105/106/107 variants to cover NASDAQ, NYSE and NYSE Arca.
func resolveAllPoolSecIDs(seed hotSeed) ([]string, error) {
	target, err := monitor.ResolveQuoteTarget(monitor.WatchlistItem{
		Symbol:   seed.Symbol,
		Market:   seed.Market,
		Currency: seed.Currency,
	})
	if err != nil {
		return nil, err
	}
	return resolveAllEastMoneySecIDs(target)
}

// hotConstituents is the predefined constituent pool for each hot category,
// used to fetch real-time data via the batch quote API.
var hotConstituents = map[monitor.HotCategory][]hotSeed{
	// ── CSI A-shares ─ top 80 most-watched A-shares ──────────────────────────
	monitor.HotCategoryCNA: {
		{Symbol: "600519", Name: "贵州茅台", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601318", Name: "中国平安", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600036", Name: "招商银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000858", Name: "五粮液", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000333", Name: "美的集团", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000651", Name: "格力电器", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601398", Name: "工商银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600900", Name: "长江电力", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600276", Name: "恒瑞医药", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601888", Name: "中国中免", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600030", Name: "中信证券", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601899", Name: "紫金矿业", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300750", Name: "宁德时代", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002594", Name: "比亚迪", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300059", Name: "东方财富", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002415", Name: "海康威视", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688981", Name: "中芯国际", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688111", Name: "金山办公", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688041", Name: "海光信息", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000001", Name: "平安银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000725", Name: "京东方A", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601919", Name: "中远海控", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601288", Name: "农业银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601988", Name: "中国银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600941", Name: "中国移动", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600809", Name: "山西汾酒", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300760", Name: "迈瑞医疗", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688256", Name: "寒武纪", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300124", Name: "汇川技术", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601818", Name: "光大银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600015", Name: "华夏银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002142", Name: "宁波银行", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601009", Name: "南京银行", Market: "CN-A", Currency: "CNY"},
		// ── Insurance / Brokers ──
		{Symbol: "601601", Name: "中国太保", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600030", Name: "中信证券", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300059", Name: "东方财富", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601688", Name: "华泰证券", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600999", Name: "招商证券", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002736", Name: "国信证券", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601377", Name: "兴业证券", Market: "CN-A", Currency: "CNY"},
		// ── New Energy / Power ──
		{Symbol: "300750", Name: "宁德时代", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002594", Name: "比亚迪", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600900", Name: "长江电力", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601012", Name: "隆基绿能", Market: "CN-A", Currency: "CNY"},
		{Symbol: "300274", Name: "阳光电源", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600089", Name: "特变电工", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601985", Name: "中国核电", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601127", Name: "赛力斯", Market: "CN-A", Currency: "CNY"},
		// ── Tech / Semiconductors ──
		{Symbol: "002415", Name: "海康威视", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688981", Name: "中芯国际", Market: "CN-A", Currency: "CNY"},
		{Symbol: "688111", Name: "金山办公", Market: "CN-A", Currency: "CNY"},

		{Symbol: "000338", Name: "潍柴动力", Market: "CN-A", Currency: "CNY"},
		// ── Transport / Automotives ──
		{Symbol: "600104", Name: "上汽集团", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601111", Name: "中国国航", Market: "CN-A", Currency: "CNY"},
		// ── Telecom ──
		{Symbol: "601728", Name: "中国电信", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600050", Name: "中国联通", Market: "CN-A", Currency: "CNY"},
		// ── Defense / Aviation ──
		{Symbol: "600760", Name: "中航沈飞", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601989", Name: "中国重工", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600031", Name: "三一重工", Market: "CN-A", Currency: "CNY"},
		{Symbol: "000157", Name: "中联重科", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601893", Name: "航发动力", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002179", Name: "中航光电", Market: "CN-A", Currency: "CNY"},
		// ── Infrastructure ──
		{Symbol: "601668", Name: "中国建筑", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601390", Name: "中国中铁", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601669", Name: "中国电建", Market: "CN-A", Currency: "CNY"},
		// ── Real Estate ──
		{Symbol: "000002", Name: "万科A", Market: "CN-A", Currency: "CNY"},
		// ── Energy / Materials ──
		{Symbol: "601857", Name: "中国石油", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600028", Name: "中国石化", Market: "CN-A", Currency: "CNY"},
		{Symbol: "601088", Name: "中国神华", Market: "CN-A", Currency: "CNY"},
		{Symbol: "002460", Name: "赣锋锂业", Market: "CN-A", Currency: "CNY"},
		{Symbol: "600309", Name: "万华化学", Market: "CN-A", Currency: "CNY"},
		// ── Healthcare / Pharma ──
		{Symbol: "300015", Name: "爱尔眼科", Market: "CN-A", Currency: "CNY"},
		{Symbol: "603259", Name: "药明康德", Market: "CN-A", Currency: "CNY"},
	},

	// ── CSI ETFs ─ top ~40 popular ETFs ──────────────────────────────────
	monitor.HotCategoryCNETF: {
		{Symbol: "510300", Name: "沪深300ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "510500", Name: "中证500ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "510050", Name: "上证50ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "588000", Name: "科创50ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159915", Name: "创业板ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159949", Name: "创业板50ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512880", Name: "证券ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "515790", Name: "光伏ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512010", Name: "医药ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512170", Name: "医疗ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159995", Name: "芯片ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "515050", Name: "5GETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512480", Name: "半导体ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "510900", Name: "H股ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "513100", Name: "纳指ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "513500", Name: "标普500ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "513180", Name: "恒生科技ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159920", Name: "恒生ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "516160", Name: "新能源ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512100", Name: "中证1000ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159901", Name: "深证100ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "510880", Name: "红利ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512690", Name: "酒ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512660", Name: "军工ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512200", Name: "房地产ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "518880", Name: "黄金ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159869", Name: "游戏ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159605", Name: "中概互联ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512800", Name: "银行ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159766", Name: "旅游ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512400", Name: "有色金属ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "515030", Name: "新能源车ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "562500", Name: "机器人ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "515180", Name: "100红利ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "512000", Name: "券商ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159611", Name: "电力ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "513060", Name: "恒生医疗ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "560080", Name: "科创芯片ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "588200", Name: "科创ETF", Market: "CN-ETF", Currency: "CNY"},
		{Symbol: "159892", Name: "储能ETF", Market: "CN-ETF", Currency: "CNY"},
	},

	// ── Hong Kong stocks ─ ~60 most-watched HK stocks ───────────────────────────────
	monitor.HotCategoryHK: {
		{Symbol: "00700", Name: "腾讯控股", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09988", Name: "阿里巴巴-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "03690", Name: "美团-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01810", Name: "小米集团-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00941", Name: "中国移动", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00005", Name: "汇丰控股", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01299", Name: "友邦保险", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02318", Name: "中国平安", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00388", Name: "香港交易所", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00939", Name: "建设银行", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01398", Name: "工商银行", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "03988", Name: "中国银行", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01211", Name: "比亚迪股份", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09618", Name: "京东集团-SW", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09888", Name: "百度集团-SW", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01024", Name: "快手-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02269", Name: "药明生物", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00883", Name: "中国海洋石油", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00688", Name: "中国海外发展", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00001", Name: "长和", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09999", Name: "网易-S", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02015", Name: "理想汽车-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09868", Name: "小鹏汽车-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09961", Name: "携程集团-S", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00027", Name: "银河娱乐", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02382", Name: "舜宇光学科技", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00669", Name: "创科实业", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01177", Name: "中国生物制药", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "06098", Name: "碧桂园服务", Market: "HK-MAIN", Currency: "HKD"},
		// ── Additional ──
		{Symbol: "02020", Name: "安踏体育", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00002", Name: "中电控股", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00003", Name: "香港中华煤气", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00006", Name: "电能实业", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00011", Name: "恒生银行", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00016", Name: "新鸿基地产", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00017", Name: "新世界发展", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00012", Name: "恒基兆业地产", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00066", Name: "港铁公司", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00101", Name: "恒隆地产", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00175", Name: "吉利汽车", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00241", Name: "阿里健康", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00268", Name: "金蝶国际", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00285", Name: "比亚迪电子", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00386", Name: "中国石油化工股份", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00762", Name: "中国联通", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00857", Name: "中国石油股份", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00868", Name: "信义玻璃", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00960", Name: "龙湖集团", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "00981", Name: "中芯国际", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01038", Name: "长江基建", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01044", Name: "恒安国际", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01109", Name: "华润置地", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01171", Name: "兖矿能源", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01347", Name: "华虹半导体", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01378", Name: "中国宏桥", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01928", Name: "金沙中国", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "01997", Name: "九龙仓置业", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02007", Name: "碧桂园", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02018", Name: "瑞声科技", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02196", Name: "复星医药", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02313", Name: "申洲国际", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02331", Name: "李宁", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02388", Name: "中银香港", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02628", Name: "中国人寿", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "02899", Name: "紫金矿业", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "06060", Name: "众安在线", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "06618", Name: "京东健康", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "06862", Name: "海底捞", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09626", Name: "哔哩哔哩-W", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09698", Name: "万国数据-SW", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09896", Name: "名创优品", Market: "HK-MAIN", Currency: "HKD"},
		{Symbol: "09901", Name: "新东方-S", Market: "HK-MAIN", Currency: "HKD"},
	},

	// ── S&P 500 ─ top ~200 most-watched constituents ────────────────────
	monitor.HotCategoryUSSP500: {
		// ── Mega-cap ──
		{Symbol: "AAPL", Name: "苹果", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSFT", Name: "微软", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NVDA", Name: "英伟达", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMZN", Name: "亚马逊", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GOOGL", Name: "谷歌A", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GOOG", Name: "谷歌C", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "META", Name: "Meta", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BRK.B", Name: "伯克希尔B", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TSLA", Name: "特斯拉", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AVGO", Name: "博通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LLY", Name: "礼来", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "JPM", Name: "摩根大通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WMT", Name: "沃尔玛", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UNH", Name: "联合健康", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "V", Name: "Visa", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MA", Name: "万事达", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "XOM", Name: "埃克森美孚", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ORCL", Name: "甲骨文", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "COST", Name: "好市多", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PG", Name: "宝洁", Market: "US-STOCK", Currency: "USD"},
		// ── Large-cap (continued) ──
		{Symbol: "HD", Name: "家得宝", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "JNJ", Name: "强生", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NFLX", Name: "奈飞", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ABBV", Name: "艾伯维", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BAC", Name: "美国银行", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CRM", Name: "赛富时", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TMUS", Name: "T-Mobile", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KO", Name: "可口可乐", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MRK", Name: "默克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CVX", Name: "雪佛龙", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PLTR", Name: "Palantir", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMD", Name: "AMD", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PEP", Name: "百事", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CSCO", Name: "思科", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TMO", Name: "赛默飞", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NOW", Name: "ServiceNow", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LIN", Name: "林德", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADBE", Name: "Adobe", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ACN", Name: "埃森哲", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ABT", Name: "雅培", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ISRG", Name: "直觉外科", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MCD", Name: "麦当劳", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GE", Name: "通用电气", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "INTU", Name: "财捷", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WFC", Name: "富国银行", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UBER", Name: "优步", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DIS", Name: "迪士尼", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TXN", Name: "德州仪器", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CAT", Name: "卡特彼勒", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "QCOM", Name: "高通", Market: "US-STOCK", Currency: "USD"},
		// ── Financials / Industrials / Semiconductors ──
		{Symbol: "GS", Name: "高盛", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MS", Name: "摩根士丹利", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BKNG", Name: "缤客", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "RTX", Name: "雷神技术", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SPGI", Name: "标普全球", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PGR", Name: "前进保险", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMGN", Name: "安进", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMAT", Name: "应用材料", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PFE", Name: "辉瑞", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DHR", Name: "丹纳赫", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VRTX", Name: "福泰制药", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ETN", Name: "伊顿", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "COP", Name: "康菲石油", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BLK", Name: "贝莱德", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AXP", Name: "美国运通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PANW", Name: "派拓网络", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LOW", Name: "劳氏", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CMCSA", Name: "康卡斯特", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NKE", Name: "耐克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SBUX", Name: "星巴克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SCHW", Name: "嘉信理财", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CB", Name: "丘博保险", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SHW", Name: "宣伟", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LMT", Name: "洛克希德马丁", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BA", Name: "波音", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DE", Name: "迪尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HON", Name: "霍尼韦尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UNP", Name: "联合太平洋", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KLAC", Name: "科磊", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "C", Name: "花旗集团", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BMY", Name: "百时美施贵宝", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GILD", Name: "吉利德科学", Market: "US-STOCK", Currency: "USD"},
		// ── Utilities / Insurance / Healthcare ──
		{Symbol: "NEE", Name: "新纪元能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CI", Name: "信诺保险", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CRWD", Name: "CrowdStrike", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SNPS", Name: "新思科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CDNS", Name: "楷登电子", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "REGN", Name: "再生元", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ICE", Name: "洲际交易所", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MMC", Name: "威达信集团", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PH", Name: "派克汉尼汾", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CME", Name: "芝商所", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MU", Name: "美光科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LRCX", Name: "泛林集团", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADI", Name: "亚德诺", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MCO", Name: "穆迪", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSCI", Name: "MSCI", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FIS", Name: "Fidelity National", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FISV", Name: "Fiserv", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ELV", Name: "Elevance Health", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TJX", Name: "TJX", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "COIN", Name: "Coinbase", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MDT", Name: "美敦力", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ABNB", Name: "爱彼迎", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SYK", Name: "史赛克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BSX", Name: "波士顿科学", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MRVL", Name: "迈威尔科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NXPI", Name: "恩智浦", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ON", Name: "安森美", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FTNT", Name: "飞塔网络", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PLD", Name: "安博", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AON", Name: "怡安保险", Market: "US-STOCK", Currency: "USD"},
		// ── Defense / Logistics / Telecom ──
		{Symbol: "NOC", Name: "诺斯罗普格鲁曼", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GD", Name: "通用动力", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TDG", Name: "TransDigm", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LHX", Name: "L3Harris", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UPS", Name: "联合包裹", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FDX", Name: "联邦快递", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CEG", Name: "星座能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "T", Name: "美国电话电报", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VZ", Name: "威瑞森", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SO", Name: "南方公司", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DUK", Name: "杜克能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PYPL", Name: "PayPal", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SQ", Name: "Block", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IBM", Name: "IBM", Market: "US-STOCK", Currency: "USD"},
		// ── Industrial Equipment / Medical Devices ──
		{Symbol: "ROP", Name: "罗珀科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EMR", Name: "艾默生电气", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ITW", Name: "伊利诺伊工具", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSI", Name: "摩托罗拉系统", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "APH", Name: "安费诺", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EW", Name: "爱德华生命科学", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ZTS", Name: "硕腾", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BDX", Name: "碧迪医疗", Market: "US-STOCK", Currency: "USD"},
		// ── Consumer / Restaurants / Hotels ──
		{Symbol: "CMG", Name: "Chipotle", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HLT", Name: "希尔顿", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MAR", Name: "万豪国际", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ORLY", Name: "奥莱利汽配", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AZO", Name: "AutoZone", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ROST", Name: "罗斯百货", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LULU", Name: "露露柠檬", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "YUM", Name: "百胜餐饮", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DPZ", Name: "达美乐", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "F", Name: "福特汽车", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GM", Name: "通用汽车", Market: "US-STOCK", Currency: "USD"},
		// ── Banks / Insurance ──
		{Symbol: "COF", Name: "第一资本", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "USB", Name: "合众银行", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PNC", Name: "PNC金融", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BK", Name: "纽约梅隆银行", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AJG", Name: "亚瑟加拉赫", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ALL", Name: "好事达", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AFL", Name: "Aflac", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PRU", Name: "保德信", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MET", Name: "大都会人寿", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TRV", Name: "旅行者保险", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HIG", Name: "哈特福德金融", Market: "US-STOCK", Currency: "USD"},
		// ── Healthcare / Pharma Distribution ──
		{Symbol: "HUM", Name: "Humana", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CVS", Name: "CVS Health", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MCK", Name: "麦克森", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CAH", Name: "康德乐", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "A", Name: "安捷伦", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IQV", Name: "IQVIA", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IDXX", Name: "爱德士", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DXCM", Name: "德康医疗", Market: "US-STOCK", Currency: "USD"},
		// ── Energy ──
		{Symbol: "EOG", Name: "EOG能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SLB", Name: "斯伦贝谢", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MPC", Name: "马拉松原油", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PSX", Name: "菲利普斯66", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VLO", Name: "瓦莱罗能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "OXY", Name: "西方石油", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DVN", Name: "德文能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FANG", Name: "Diamondback能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HAL", Name: "哈里伯顿", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WMB", Name: "威廉姆斯", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KMI", Name: "金德摩根", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "OKE", Name: "Oneok", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TRGP", Name: "Targa Resources", Market: "US-STOCK", Currency: "USD"},
		// ── Utilities / Materials ──
		{Symbol: "D", Name: "道明尼能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AEP", Name: "美国电力", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SRE", Name: "森普拉能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PCG", Name: "太平洋煤电", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "APD", Name: "空气化工", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ECL", Name: "艺康", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DD", Name: "杜邦", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PPG", Name: "PPG工业", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FCX", Name: "自由港麦克莫兰", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NEM", Name: "纽蒙特矿业", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NUE", Name: "纽柯钢铁", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DOW", Name: "陶氏化学", Market: "US-STOCK", Currency: "USD"},
		// ── REITs / Real Estate ──
		{Symbol: "AMT", Name: "美国电塔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CCI", Name: "冠城国际", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EQIX", Name: "Equinix", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SPG", Name: "西蒙地产", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PSA", Name: "公共仓储", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "O", Name: "Realty Income", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DLR", Name: "Digital Realty", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WELL", Name: "Welltower", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CBRE", Name: "世邦魏理仕", Market: "US-STOCK", Currency: "USD"},
		// ── Industrials / Transport / Waste ──
		{Symbol: "URI", Name: "联合租赁", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WM", Name: "废物管理", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "RSG", Name: "共和服务", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ROK", Name: "罗克韦尔自动化", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SWK", Name: "史丹利百得", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "OTIS", Name: "奥的斯电梯", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CARR", Name: "开利", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AME", Name: "阿美特克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DAL", Name: "达美航空", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UAL", Name: "联合航空", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LUV", Name: "西南航空", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CSX", Name: "CSX运输", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NSC", Name: "诺福克南方", Market: "US-STOCK", Currency: "USD"},
		// ── Gaming / Tech Services / Other ──
		{Symbol: "LVS", Name: "金沙集团", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WYNN", Name: "永利度假村", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IT", Name: "Gartner", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HPQ", Name: "惠普", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DELL", Name: "戴尔科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KEYS", Name: "是德科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GEN", Name: "Gen Digital", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "INTC", Name: "英特尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CPRT", Name: "Copart", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CTAS", Name: "信达思", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PAYX", Name: "Paychex", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ODFL", Name: "Old Dominion", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EFX", Name: "Equifax", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VRSK", Name: "Verisk", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BR", Name: "Broadridge", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IRM", Name: "Iron Mountain", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MMM", Name: "3M", Market: "US-STOCK", Currency: "USD"},
	},

	// ── NASDAQ-100 ─ all ~100 NASDAQ-100 constituents (2024/2025) ──────
	monitor.HotCategoryUSNasdaq: {
		{Symbol: "AAPL", Name: "苹果", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSFT", Name: "微软", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMZN", Name: "亚马逊", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NVDA", Name: "英伟达", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GOOGL", Name: "谷歌A", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GOOG", Name: "谷歌C", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "META", Name: "Meta", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TSLA", Name: "特斯拉", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AVGO", Name: "博通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "COST", Name: "好市多", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NFLX", Name: "奈飞", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ASML", Name: "阿斯麦", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AZN", Name: "阿斯利康", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMD", Name: "AMD", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADBE", Name: "Adobe", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PEP", Name: "百事", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CSCO", Name: "思科", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TMUS", Name: "T-Mobile", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "INTU", Name: "财捷", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "QCOM", Name: "高通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CMCSA", Name: "康卡斯特", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMGN", Name: "安进", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ARM", Name: "Arm", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ISRG", Name: "直觉外科", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PLTR", Name: "Palantir", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BKNG", Name: "缤客", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "APP", Name: "AppLovin", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMAT", Name: "应用材料", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADI", Name: "亚德诺", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADP", Name: "自动数据处理", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PANW", Name: "派拓网络", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FICO", Name: "FICO", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LRCX", Name: "泛林集团", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MU", Name: "美光科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GILD", Name: "吉利德科学", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KLAC", Name: "科磊", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SNPS", Name: "新思科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CDNS", Name: "楷登电子", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "INTC", Name: "英特尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MELI", Name: "MercadoLibre", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CRWD", Name: "CrowdStrike", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PDD", Name: "拼多多", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SBUX", Name: "星巴克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CEG", Name: "星座能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PYPL", Name: "PayPal", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MDLZ", Name: "亿滋国际", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "REGN", Name: "再生元", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NXPI", Name: "恩智浦", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MRVL", Name: "迈威尔科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CTAS", Name: "信达思", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ABNB", Name: "爱彼迎", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MAR", Name: "万豪国际", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HON", Name: "霍尼韦尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ORLY", Name: "奥莱利汽配", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FTNT", Name: "飞塔网络", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DASH", Name: "DoorDash", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WDAY", Name: "Workday", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSTR", Name: "MicroStrategy", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PCAR", Name: "帕卡", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CHTR", Name: "Charter通信", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ROP", Name: "Roper Technologies", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TTD", Name: "交易台", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ADSK", Name: "欧特克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CPRT", Name: "Copart", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AXON", Name: "Axon", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MNST", Name: "怪物饮料", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KDP", Name: "Keurig Dr Pepper", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DDOG", Name: "Datadog", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PAYX", Name: "沛齐", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EA", Name: "EA", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ROST", Name: "罗斯百货", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ON", Name: "安森美", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FAST", Name: "快扣", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VRSK", Name: "Verisk", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IDXX", Name: "IDEXX", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CTSH", Name: "高知特", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GEHC", Name: "GE医疗", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ODFL", Name: "Old Dominion", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ANSS", Name: "Ansys", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TTWO", Name: "Take-Two", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BIIB", Name: "渤健", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "LULU", Name: "露露柠檬", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DXCM", Name: "德康医疗", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KHC", Name: "卡夫亨氏", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MCHP", Name: "微芯科技", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TEAM", Name: "Atlassian", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ZS", Name: "Zscaler", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MDB", Name: "MongoDB", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CDW", Name: "CDW", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CCEP", Name: "可口可乐欧洲太平洋", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "FANG", Name: "Diamondback能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BKR", Name: "贝克休斯", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DLTR", Name: "Dollar Tree", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GFS", Name: "格芯", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ALGN", Name: "阿莱技术", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MRNA", Name: "Moderna", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "XEL", Name: "Xcel能源", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "EXC", Name: "Exelon", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AEP", Name: "美国电力", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SMCI", Name: "超微电脑", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ILMN", Name: "Illumina", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "ENPH", Name: "Enphase能源", Market: "US-STOCK", Currency: "USD"},
	},

	// ── Dow Jones 30 ─ all 30 Dow constituents (+ recent additions) ────────
	monitor.HotCategoryUSDow: {
		{Symbol: "AAPL", Name: "苹果", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MSFT", Name: "微软", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMZN", Name: "亚马逊", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NVDA", Name: "英伟达", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "UNH", Name: "联合健康", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "V", Name: "Visa", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "JPM", Name: "摩根大通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HD", Name: "家得宝", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "PG", Name: "宝洁", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "JNJ", Name: "强生", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CRM", Name: "赛富时", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MRK", Name: "默克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CVX", Name: "雪佛龙", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "KO", Name: "可口可乐", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CAT", Name: "卡特彼勒", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AXP", Name: "美国运通", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MCD", Name: "麦当劳", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "GS", Name: "高盛", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DIS", Name: "迪士尼", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "BA", Name: "波音", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "AMGN", Name: "安进", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "IBM", Name: "IBM", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "HON", Name: "霍尼韦尔", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "NKE", Name: "耐克", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "SHW", Name: "宣伟", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "MMM", Name: "3M", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "TRV", Name: "旅行者保险", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "DOW", Name: "陶氏化学", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "VZ", Name: "威瑞森", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "CSCO", Name: "思科", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "WMT", Name: "沃尔玛", Market: "US-STOCK", Currency: "USD"},
		{Symbol: "INTC", Name: "英特尔", Market: "US-STOCK", Currency: "USD"},
	},

	// ── HK-listed ETFs ─ top 20 popular HK-listed ETFs ────────────────────────
	monitor.HotCategoryHKETF: {
		{Symbol: "02800", Name: "盈富基金", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02828", Name: "恒生中国企业", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03033", Name: "南方恒生科技", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03067", Name: "安硕恒生科技", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02823", Name: "安硕A50中国", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03188", Name: "华夏沪深300", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02836", Name: "安硕印度", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03081", Name: "价值黄金", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03088", Name: "华夏恒生科技", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03032", Name: "恒生科技ETF", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02840", Name: "SPDR金ETF", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03037", Name: "安硕恒生指数", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02801", Name: "安硕MSCI中国", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03007", Name: "安硕纳指100", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "09834", Name: "安硕标普500", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03115", Name: "安硕恒生指数", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "09077", Name: "安硕恒生科技", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "02833", Name: "恒指ETF", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03101", Name: "安硕A50中国", Market: "HK-ETF", Currency: "HKD"},
		{Symbol: "03147", Name: "南方恒生红利", Market: "HK-ETF", Currency: "HKD"},
	},

	// ── US-listed ETFs ─ ~80 popular US-listed ETFs ───────────────────────────
	monitor.HotCategoryUSETF: {
		// ── Broad-market Indices ──
		{Symbol: "SPY", Name: "SPDR标普500ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "QQQ", Name: "纳指100ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IVV", Name: "iShares标普500", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VOO", Name: "Vanguard标普500", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VTI", Name: "Vanguard全美股票", Market: "US-ETF", Currency: "USD"},
		{Symbol: "DIA", Name: "道琼斯工业ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IWM", Name: "罗素2000小盘", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IWF", Name: "罗素1000成长", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IWD", Name: "罗素1000价值", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VTV", Name: "Vanguard大盘价值", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VUG", Name: "Vanguard大盘成长", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IJH", Name: "iShares中盘核心", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IJR", Name: "iShares小盘核心", Market: "US-ETF", Currency: "USD"},
		{Symbol: "RSP", Name: "标普500等权重", Market: "US-ETF", Currency: "USD"},
		// ── Sector SPDRs ──
		{Symbol: "XLF", Name: "金融精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLK", Name: "科技精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLE", Name: "能源精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLV", Name: "医疗精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLI", Name: "工业精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLP", Name: "必需消费SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLY", Name: "非必需消费SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLC", Name: "通信服务SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLU", Name: "公用事业SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLB", Name: "材料精选SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "XLRE", Name: "房地产SPDR", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VGT", Name: "Vanguard信息科技", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VNQ", Name: "Vanguard房地产", Market: "US-ETF", Currency: "USD"},
		// ── Thematic / Sector ──
		{Symbol: "ARKK", Name: "ARK创新ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SOXX", Name: "iShares半导体", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SMH", Name: "VanEck半导体", Market: "US-ETF", Currency: "USD"},
		{Symbol: "KWEB", Name: "中概互联网ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "FXI", Name: "中国大盘股ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "HACK", Name: "网络安全ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "LIT", Name: "锂电池科技ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "TAN", Name: "太阳能ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "ICLN", Name: "iShares清洁能源", Market: "US-ETF", Currency: "USD"},
		{Symbol: "BITO", Name: "ProShares比特币期货", Market: "US-ETF", Currency: "USD"},
		// ── Fixed Income ──
		{Symbol: "TLT", Name: "20年+美国国债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IEF", Name: "7-10年美国国债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SHY", Name: "1-3年美国国债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "BND", Name: "Vanguard总债券", Market: "US-ETF", Currency: "USD"},
		{Symbol: "AGG", Name: "iShares总债券", Market: "US-ETF", Currency: "USD"},
		{Symbol: "LQD", Name: "投资级公司债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "HYG", Name: "iShares高收益债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "JNK", Name: "SPDR高收益债", Market: "US-ETF", Currency: "USD"},
		{Symbol: "TIP", Name: "通胀保值债券", Market: "US-ETF", Currency: "USD"},
		{Symbol: "EMB", Name: "新兴市场美元债", Market: "US-ETF", Currency: "USD"},
		// ── Commodities ──
		{Symbol: "GLD", Name: "SPDR黄金ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IAU", Name: "iShares黄金信托", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SLV", Name: "iShares白银", Market: "US-ETF", Currency: "USD"},
		{Symbol: "GDX", Name: "金矿股ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "GDXJ", Name: "小型金矿股ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "USO", Name: "美国原油ETF", Market: "US-ETF", Currency: "USD"},
		{Symbol: "UNG", Name: "美国天然气ETF", Market: "US-ETF", Currency: "USD"},
		// ── International / Regional ──
		{Symbol: "EEM", Name: "iShares新兴市场", Market: "US-ETF", Currency: "USD"},
		{Symbol: "EMXC", Name: "iShares MSCI新兴市场除中国", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VEA", Name: "Vanguard发达市场", Market: "US-ETF", Currency: "USD"},
		{Symbol: "VWO", Name: "Vanguard新兴市场", Market: "US-ETF", Currency: "USD"},
		{Symbol: "EFA", Name: "iShares发达市场", Market: "US-ETF", Currency: "USD"},
		{Symbol: "IEMG", Name: "iShares核心新兴市场", Market: "US-ETF", Currency: "USD"},
		{Symbol: "INDA", Name: "iShares印度", Market: "US-ETF", Currency: "USD"},
		{Symbol: "EWJ", Name: "iShares日本", Market: "US-ETF", Currency: "USD"},
		{Symbol: "EWZ", Name: "iShares巴西", Market: "US-ETF", Currency: "USD"},
		// ── Leveraged / Inverse ──
		{Symbol: "TQQQ", Name: "纳指3倍做多", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SQQQ", Name: "纳指3倍做空", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SPXL", Name: "标普500三倍做多", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SOXL", Name: "半导体3倍做多", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SOXS", Name: "半导体3倍做空", Market: "US-ETF", Currency: "USD"},
		{Symbol: "UVXY", Name: "1.5倍做多波动率", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SH", Name: "标普500反向", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SDS", Name: "标普500两倍做空", Market: "US-ETF", Currency: "USD"},
		// ── Dividends / Income ──
		{Symbol: "VYM", Name: "Vanguard高股息", Market: "US-ETF", Currency: "USD"},
		{Symbol: "SCHD", Name: "Schwab美国股息", Market: "US-ETF", Currency: "USD"},
		{Symbol: "DVY", Name: "iShares精选股息", Market: "US-ETF", Currency: "USD"},
		{Symbol: "HDV", Name: "iShares高股息", Market: "US-ETF", Currency: "USD"},
		{Symbol: "DGRO", Name: "iShares股息增长", Market: "US-ETF", Currency: "USD"},
		{Symbol: "JEPI", Name: "摩根股票溢价收益", Market: "US-ETF", Currency: "USD"},
		{Symbol: "JEPQ", Name: "摩根纳指溢价收益", Market: "US-ETF", Currency: "USD"},
		// ── Factor / Smart Beta ──
		{Symbol: "MTUM", Name: "iShares动量因子", Market: "US-ETF", Currency: "USD"},
		{Symbol: "QUAL", Name: "iShares质量因子", Market: "US-ETF", Currency: "USD"},
		{Symbol: "MOAT", Name: "VanEck宽护城河", Market: "US-ETF", Currency: "USD"},
	},
}
