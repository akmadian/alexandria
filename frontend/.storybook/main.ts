import type { StorybookConfig } from "@storybook/react-vite";

// Stories live beside their component (src/components/<name>/<name>.stories.tsx),
// matching the repo's colocation convention. The react-vite framework reuses
// vite.config.ts, so the "@/" alias and the token-enforcement stylelint plugin
// come along for free. addon-mcp serves the component inventory to coding agents
// (catalog + pattern enforcement); addon-vitest (real-browser test loop) and
// Chromatic (visual regression) are deferred until wanted.
const config: StorybookConfig = {
    stories: ["../src/**/*.stories.@(ts|tsx)"],
    addons: ["@storybook/addon-docs", "@storybook/addon-a11y", "@storybook/addon-mcp"],
    framework: "@storybook/react-vite",
};

export default config;
