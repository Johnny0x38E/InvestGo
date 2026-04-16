# InvestGo

[English](./README.md) | [简体中文](./README.zh-CN.md) | [许可证](./LICENSE)

InvestGo 是一个基于 Go 和 Wails v3 的桌面投资观察应用，支持自选标的、实时行情、历史走势、热门榜单和价格提醒。

## 截图

![Light](assets/screenshot1-light.png)

![Dark](assets/screenshot2-dark.png)

![Screenshot 2](assets/screenshot3.png)

![Screenshot 3](assets/screenshot4.png)

![Screenshot 4](assets/screenshot5.png)

![Screenshot 5](assets/screenshot6.png)

## 快速开始

```bash
git clone https://github.com/Johnny0x38E/InvestGo.git
cd InvestGo
npm install
npm run dev           # 前端开发服务 (localhost:5173)
go run main.go -dev   # 后端开发
```

## 构建

```bash
VERSION=0.1.0 ./scripts/build-macos-arm64.sh
./scripts/build-macos-arm64.sh --dev
./scripts/package-macos-dmg.sh
```

## 环境要求

- Go 1.25+
- Node.js 20+
- macOS arm64

## 免责声明

**重要提示**：本软件仅用于个人学习和投资观察目的，不构成任何形式的投资建议、财务建议或买卖建议。

对于本软件提供的所有数据、信息和功能，用户应自行判断并核验其准确性和完整性。作者和贡献者不对以下情况承担任何责任：

1. 因使用本软件而产生的任何投资损失或收益
2. 本软件所提供数据的准确性、及时性或完整性
3. 因网络故障、数据源变更或其他技术问题导致的数据中断或错误
4. 任何基于本软件信息做出的投资决策及其结果

投资有风险，入市需谨慎。用户在使用本软件前应充分了解投资风险，并自行承担所有投资决策的后果。

## 许可证

本项目基于 [MIT License](./LICENSE) 开源。
