# InvestGo

InvestGo 是一个面向个人投资观察的桌面应用，基于 Go 1.25、Wails v3、Vue 3、TypeScript 和 PrimeVue 构建，主要针对 macOS arm64。

它提供自选标的管理、实时行情查看、历史走势、热门榜单和价格提醒，适合用来做日常投资观察与记录。

## Screenshots

![Light](assets/screenshot-light.png)

![Dark](assets/screenshot-dark.png)

![Screenshot 2](assets/screenshot2.png)

![Screenshot 3](assets/screenshot3.png)

![Screenshot 4](assets/screenshot4.png)

![Screenshot 5](assets/screenshot5.png)

## Clone

```bash
git clone https://github.com/Johnny0x38E/InvestGo.git
cd InvestGo
```

## Environment

开始前请准备：

- Go 1.25+
- Node.js 20+
- npm 10+
- macOS arm64
- Wails v3 构建所需的本机开发环境

安装依赖：

```bash
npm install
```

## Run

前端开发：

```bash
npm run dev
```

后端开发：

```bash
go run main.go -dev
```

常用检查：

```bash
npm run typecheck
env GOCACHE=/tmp/go-build-cache go test ./...
```

## Build

构建前端：

```bash
npm run build
```

构建 macOS arm64 二进制：

```bash
./scripts/build-macos-arm64.sh
```

构建带 DevTools 的调试二进制：

```bash
./scripts/build-macos-arm64.sh --dev
```

打包 `.app` 和 `.dmg`：

```bash
./scripts/package-macos-dmg.sh
```

打包带 DevTools 的调试版：

```bash
./scripts/package-macos-dmg.sh --dev
```

## Disclaimer

### 中文

**重要提示**：本软件仅用于个人学习和投资观察目的，不构成任何形式的投资建议、财务建议或买卖建议。

使用本软件所提供的所有数据、信息和功能，用户应当自行判断其准确性和完整性。作者和贡献者不对以下情况承担任何责任：

1. 因使用本软件而产生的任何投资损失或收益；
2. 本软件所提供数据的准确性、及时性或完整性；
3. 因网络故障、数据源变更或其他技术问题导致的数据中断或错误；
4. 任何基于本软件信息做出的投资决策的结果。

投资有风险，入市需谨慎。用户在使用本软件前应充分了解投资风险，并自行承担所有投资决策的后果。

### English

**IMPORTANT NOTICE**: This software is intended for personal learning and investment observation purposes only and does not constitute any form of investment advice, financial advice, or recommendation to buy or sell.

Users should independently verify the accuracy and completeness of all data, information, and functions provided by this software. The authors and contributors assume no liability for:

1. Any investment losses or gains resulting from the use of this software;
2. The accuracy, timeliness, or completeness of the data provided;
3. Data interruptions or errors caused by network failures, data source changes, or other technical issues;
4. Any outcomes from investment decisions based on information from this software.

Investment involves risks. Users should fully understand the investment risks before using this software and assume full responsibility for all consequences of their investment decisions.

## License

MIT
