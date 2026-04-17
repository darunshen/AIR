# 发布与安装包交付

本文档描述 AIR 当前的安装包交付方式。

当前目标是先打通：

- GitHub Release 二进制产物
- `.deb` 安装包
- apt 仓库目录产物

其中：

- GitHub Release 和 `.deb` 已可直接交付
- apt 仓库当前先生成目录与压缩包
- 仓库签名、长期托管、`apt` 安装源域名可以下一步再补

## 1. 当前产物

运行：

```bash
./scripts/build-release-artifacts.sh dist
```

会生成：

- `air_<version>_linux_amd64.tar.gz`
- `air_<version>_linux_arm64.tar.gz`
- `air_<version>_darwin_amd64.tar.gz`
- `air_<version>_darwin_arm64.tar.gz`
- `air_<version>_windows_amd64.zip`
- `air_<version>_amd64.deb`
- `air_<version>_arm64.deb`
- `air_<version>_apt-repo.tar.gz`
- `checksums.txt`
- `BUILD_INFO.txt`

每个归档中都包含：

- `air`
- `air-agent`
- `README.md`
- `LICENSE`

## 2. 版本信息

当前版本信息通过 linker flags 注入，支持：

```bash
air version
air-agent --version
```

## 3. 本地构建

默认从 git 自动推断版本：

```bash
./scripts/build-release-artifacts.sh dist
```

也可以显式指定：

```bash
VERSION=v0.1.0 \
COMMIT=$(git rev-parse --short HEAD) \
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
./scripts/build-release-artifacts.sh dist
```

## 4. GitHub Release

仓库已提供发布工作流：

- [.github/workflows/release.yml](../.github/workflows/release.yml)

触发方式：

- 推送 tag，例如 `v0.1.0`
- 或手动 `workflow_dispatch`

工作流会：

1. 构建跨平台归档
2. 构建 Linux `.deb`
3. 生成 apt 仓库目录压缩包
4. 上传到 GitHub Release

## 5. `.deb` 安装

从 GitHub Release 下载后可直接安装：

```bash
sudo dpkg -i air_<version>_amd64.deb
```

安装内容：

- `/usr/bin/air`
- `/usr/bin/air-agent`
- `/usr/share/doc/air/README.md`
- `/usr/share/doc/air/LICENSE`

## 6. apt 仓库目录

当前会生成：

```text
dist/apt-repo/
  pool/
  dists/
```

以及：

```text
dist/air_<version>_apt-repo.tar.gz
```

这一步主要用于先把 apt 仓库格式的目录产物构建出来。

当前还没有做：

- GPG 签名
- GitHub Pages / 独立域名托管
- `apt` 源安装文档中的正式 `signed-by` 配置

因此这一阶段更适合：

- 先把 `.deb` 放到 GitHub Release
- 先把 apt repo 目录作为发布产物保留
- 下一步再决定是否上 GitHub Pages 或独立对象存储

## 7. 下一步建议

建议按这个顺序推进：

1. 先用 GitHub Release 交付 `.tar.gz`、`.zip`、`.deb`
2. 再给 apt repo 增加签名
3. 再把 apt repo 发布到固定地址
4. 最后补安装脚本和正式安装文档
