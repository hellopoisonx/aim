---
name: aim-repo-mapping
description: aim 仓库导航 Skill。用于快速判断需求应该落到哪个业务域、哪个控制器、哪个工作流或哪个配置文件；当需求入口不清晰、需要先找模块边界、接口总览、运行入口或改动影响面时使用。
---
# aim-repo-mapping

先用这层做路由，再切到对应业务 Skill。

## 使用顺序

- 先看 `references/request-to-module-map.md`，把需求归到一个主域。
- 再看 `references/module-routing.md`，找到对应的模块。
- 如果需求涉及改接口、改配置、改工作流或跨多个域，立刻切到对应领域 Skill，不要长期停留在总路由层。

## 路由原则

- 先判断对象是什么，再判断它属于哪个域。

## 参考资料

- `references/module-routing.md`
- `references/request-to-module-map.md`