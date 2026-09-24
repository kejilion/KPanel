# 场景包起步模板

一个不依赖任何库的最小场景：一个全屏 WebGL2 着色器，完整实现了宿主协议（`ready` / `camera` / `error`，
`pause` / `resume` / `camera`），并演示了进场（从黑场淡入）、待机呼吸和机位缓动转场。
目录名以 `_` 开头，所以构建与校验都会跳过它，不会出现在面板里。

1. 把整个目录复制为 `scene-packs/<你的 ID>/`，把 `manifest.json` 的 `id` 改成同一个 ID；
2. 修改 `manifest.json` 的名称、描述、作者、`theme` 与 `cameras`，`src/main.ts` 里的 `SHOTS` 与之保持一致；
3. 把着色器换成你的世界，或者整个换成 Three.js / Babylon.js / 你自己的引擎；
4. 运行 `npm --prefix web run scene-packs:build`，打开 `dist/index.html?entrance=off&shot=0`，截一张 1664×936
   的画面做 `poster.webp`，再缩成 400×225 做 `thumb.webp`（放在包目录下，重新构建）；
5. 运行 `node scripts/check-scene-packs.mjs`，通过后提交 Pull Request。

完整规范见 [`scene-packs/README.md`](../README.md)。
