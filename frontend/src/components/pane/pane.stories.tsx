import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, waitFor } from "storybook/test";
import { Button } from "@/components/button/button";
import { Tree, type TreeNodeData } from "@/components/tree/tree";
import { Pane } from "./pane";

// Pane — the §12 docked side container (browser rail / inspector rail): drag the edge seam to
// resize, collapse to hidden, width + collapsed persist. Docked chrome — flat, one hairline seam.
// Stories dock it in a full-height frame beside a filler "well" so the resize + collapse read.
const meta = {
    title: "Layout/Pane",
    component: Pane,
    // children is supplied by each story's render (the docked frame + rail contents).
    args: { side: "left", "aria-label": "Sources", defaultWidth: 280, minWidth: 200, maxWidth: 480, children: null },
    argTypes: {
        side: { control: "inline-radio", options: ["left", "right"] },
        defaultWidth: { control: { type: "number" } },
        minWidth: { control: { type: "number" } },
        maxWidth: { control: { type: "number" } },
        defaultCollapsed: { control: "boolean" },
    },
} satisfies Meta<typeof Pane>;

export default meta;

type Story = StoryObj<typeof meta>;

const folders: TreeNodeData[] = [
    {
        id: "mac",
        icon: "source",
        count: 48213,
        label: "Macintosh HD",
        children: [
            {
                id: "2024",
                icon: "folder",
                count: 412,
                label: "2024",
                children: [
                    { id: "iceland", icon: "folder", count: 88, label: "Iceland" },
                    { id: "japan", icon: "folder", count: 20300, label: "Japan" },
                ],
            },
            { id: "2023", icon: "folder", count: 1204, label: "2023" },
        ],
    },
];

// A docked frame: the pane on its side, a filler "well" filling the rest — so resizing the pane
// visibly steals/gives space to the center, exactly as it will in the shell.
function Frame({ side, children }: { side: "left" | "right"; children: React.ReactNode }) {
    const well = (
        <div
            style={{
                flex: 1,
                minWidth: 0,
                background: "var(--alx-cell-well)",
                display: "grid",
                placeItems: "center",
                color: "var(--alx-ink-3)",
            }}
            className="alx-type-label"
        >
            content well
        </div>
    );
    return (
        <div style={{ display: "flex", height: 360, background: "var(--alx-surface-panel)" }}>
            {side === "left" ? children : well}
            {side === "left" ? well : children}
        </div>
    );
}

function FolderTree() {
    return (
        <div style={{ padding: "var(--alx-space-2) 0", height: "100%", overflow: "auto" }}>
            <Tree aria-label="Folders" nodes={folders} defaultExpandedKeys={["mac", "2024"]} defaultSelectedKeys={["iceland"]} />
        </div>
    );
}

// The left browser rail: a tree docked left, resize seam on the right edge.
export const Left: Story = {
    args: { side: "left", "aria-label": "Sources", defaultWidth: 260 },
    render: (args) => (
        <Frame side="left">
            <Pane {...args}>
                <FolderTree />
            </Pane>
        </Frame>
    ),
};

// The right inspector rail: resize seam on the left edge, delta sign flips.
export const Right: Story = {
    args: { side: "right", "aria-label": "Inspector", defaultWidth: 300 },
    render: (args) => (
        <Frame side="right">
            <Pane {...args}>
                <FolderTree />
            </Pane>
        </Frame>
    ),
};

// Collapsed at rest (uncontrolled default): the body is hidden, the well takes the whole width,
// the grip stays at the edge to reveal it again.
export const Collapsed: Story = {
    args: { side: "left", "aria-label": "Sources", defaultCollapsed: true },
    render: (args) => (
        <Frame side="left">
            <Pane {...args}>
                <FolderTree />
            </Pane>
        </Frame>
    ),
};

// Controlled collapse driven by an EXTERNAL toggle (the workspace-header pattern). The play
// function drives the toggle and asserts the pane hides, plus the callback fires.
export const ExternalToggle: Story = {
    args: { onCollapsedChange: fn() },
    render: (args) => {
        const [collapsed, setCollapsed] = useState(false);
        return (
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--alx-space-2)" }}>
                <Button
                    onPress={() => {
                        setCollapsed((previous) => !previous);
                        args.onCollapsedChange?.(!collapsed);
                    }}
                >
                    Toggle sidebar
                </Button>
                <Frame side="left">
                    <Pane {...args} isCollapsed={collapsed}>
                        <FolderTree />
                    </Pane>
                </Frame>
            </div>
        );
    },
    play: async ({ canvas, args }) => {
        const pane = canvas.getByRole("region", { name: "Sources" });
        await expect(pane).not.toHaveAttribute("data-collapsed");
        await userEvent.click(canvas.getByRole("button", { name: "Toggle sidebar" }));
        await waitFor(() => expect(pane).toHaveAttribute("data-collapsed"));
        await expect(args.onCollapsedChange).toHaveBeenCalledWith(true);
    },
};

// Keyboard-driven resize + collapse (the APG Window Splitter contract): focus the separator,
// ArrowRight grows a left pane, Enter collapses it.
export const KeyboardResize: Story = {
    args: { side: "left", "aria-label": "Sources", defaultWidth: 260 },
    render: (args) => (
        <Frame side="left">
            <Pane {...args}>
                <FolderTree />
            </Pane>
        </Frame>
    ),
    play: async ({ canvas }) => {
        const handle = canvas.getByRole("separator", { name: "Resize Sources" });
        await expect(handle).toHaveAttribute("aria-valuenow", "260");
        handle.focus();
        await userEvent.keyboard("{ArrowRight}{ArrowRight}");
        await waitFor(() => expect(Number(handle.getAttribute("aria-valuenow"))).toBeGreaterThan(260));
        await userEvent.keyboard("{Enter}");
        await waitFor(() => expect(handle).toHaveAttribute("aria-valuenow", "0"));
    },
};

// Crash isolation: a throwing child is caught by the pane's built-in PaneErrorBoundary — the
// crash never escapes the container. Consumers get this for free.
function Bomb(): React.ReactNode {
    throw new Error("boom");
}

export const ContentCrash: Story = {
    args: { side: "left", "aria-label": "Sources" },
    render: (args) => (
        <Frame side="left">
            <Pane {...args}>
                <Bomb />
            </Pane>
        </Frame>
    ),
    play: async ({ canvas }) => {
        await expect(canvas.getByRole("button", { name: "Reload panel" })).toBeInTheDocument();
    },
};
