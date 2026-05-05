/**
 * 初始化 Pure White 主题的顶部图片收起按钮。
 *
 * 功能：
 * - 查找主题模板插入的 `.hero-collapse-button`。
 * - 点击后把导航栏滚动到视口顶部，相当于把顶部封面图移出视口。
 *
 * 设计说明：
 * - 这段脚本属于主题包自身，不依赖全局 site.js 修改。
 * - 使用 `scrollIntoView` 可以避免硬编码封面高度，移动端和桌面端都能适配。
 * - 如果用户系统开启“减少动态效果”，滚动会自动切换为无动画。
 */
function setupPureWhiteHeroCollapseButton() {
  const button = document.querySelector(".hero-collapse-button");
  if (!button) return;

  button.addEventListener("click", () => {
    const nav = button.closest(".section-nav");
    if (!nav) return;

    const prefersReducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    nav.scrollIntoView({
      block: "start",
      behavior: prefersReducedMotion ? "auto" : "smooth"
    });
  });
}

document.addEventListener("DOMContentLoaded", setupPureWhiteHeroCollapseButton);
