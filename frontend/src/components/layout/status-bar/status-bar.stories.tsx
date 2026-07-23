import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { StatusBar } from "./status-bar";

// StatusBar — the §12 bottom readout band. Docked chrome, one line, data-sm mono. A single content
// slot: the feature composes the three lanes (counts · filename · machinery) with a margin-auto
// spacer. Stories dock it under a content well so the seam + band height read as they will in-app.
const meta = {
    title: "Layout/StatusBar",
    component: StatusBar,
    args: { "aria-label": "Status", children: null },
    decorators: [
        (Story) => (
            <div
                style={{
                    display: "flex",
                    flexDirection: "column",
                    height: 240,
                    width: 720,
                    background: "var(--alx-surface-panel)",
                    border: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                    borderRadius: "var(--alx-radius-docked)",
                }}
            >
                <div style={{ flex: 1, minHeight: 0, background: "var(--alx-cell-well)" }} />
                <Story />
            </div>
        ),
    ],
} satisfies Meta<typeof StatusBar>;

export default meta;

type Story = StoryObj<typeof meta>;

// A tiny spacer that eats the free space, pushing what follows to the trailing edge (§12 right zone).
const spacer = <span style={{ marginLeft: "auto" }} />;

// The full readout (§15): counts at the start, the active filename after, machinery at the end.
export const Readout: Story = {
    render: (args) => (
        <StatusBar {...args}>
            <span>1,204 assets</span>
            <span>3 selected</span>
            <span style={{ color: "var(--alx-ink-1)" }}>_DSF4926.RAF</span>
            {spacer}
            <span>Ingesting 42…</span>
        </StatusBar>
    ),
};

// Empty selection is legal (§15) — the readout drops to the source summary alone.
export const CountOnly: Story = {
    render: (args) => (
        <StatusBar {...args}>
            <span>1,204 assets</span>
        </StatusBar>
    ),
};

// Transient fan-out confirmation after a batch verb (§15: `★★★ → 3 assets`). Feature-owned content;
// the shell just carries it.
export const FanOutConfirmation: Story = {
    render: (args) => (
        <StatusBar {...args}>
            <span>1,204 assets</span>
            <span>3 selected</span>
            {spacer}
            <span style={{ color: "var(--alx-ink-1)" }}>★★★ → 3 assets</span>
        </StatusBar>
    ),
};

// Overflow clips to one line — a long filename never wraps or grows the band.
export const Overflow: Story = {
    render: (args) => (
        <StatusBar {...args}>
            <span>48,213 assets</span>
            <span>1 selected</span>
            <span style={{ color: "var(--alx-ink-1)" }}>
                a-very-long-original-capture-filename-that-would-wrap-if-the-band-let-it-2026-07-22.RAF
            </span>
            {spacer}
            <span>Thumbnailing 8…</span>
        </StatusBar>
    ),
    play: async ({ canvasElement }) => {
        const bar = canvasElement.querySelector("footer")!;
        // One line, always: the band's height stays on the row-list rung despite the long name.
        await expect(bar.clientHeight).toBeLessThanOrEqual(25);
    },
};
