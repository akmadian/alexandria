import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tree, type TreeNodeData } from "./tree";

// Tree — the §12 browser rail. Docked chrome (flat, no §6 shell), items-driven so the elbow
// connectors can be computed from the sibling/last-child structure. One component across
// Sources / Collections / Tags — different data + kind icon. Specimens sit on a surface.panel
// rail (the decorator) with a single seam, as it docks in the app.
const meta = {
    title: "Primitives/Tree",
    component: Tree,
    args: { "aria-label": "Tree", nodes: [] },
    decorators: [
        (Story) => (
            <div
                style={{
                    width: "var(--alx-size-panel-right)",
                    minHeight: 340,
                    padding: "var(--alx-space-2) 0",
                    borderRight: "var(--alx-stroke-hairline) solid var(--alx-ink-hairline)",
                    background: "var(--alx-surface-panel)",
                }}
            >
                <Story />
            </div>
        ),
    ],
} satisfies Meta<typeof Tree>;

export default meta;

type Story = StoryObj<typeof meta>;

// Folders / Sources — a volume root over a folder hierarchy; the deep nesting makes the elbow
// connectors and the last-child (└) read. Selection shows via stroke weight + ink, not fill.
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
                    { id: "long", icon: "folder", count: 7, label: "A very long folder name that must end-truncate" },
                ],
            },
            { id: "2023", icon: "folder", count: 1204, label: "2023" },
        ],
    },
];

export const Folders: Story = {
    render: () => (
        <Tree aria-label="Folders" nodes={folders} defaultExpandedKeys={["mac", "2024"]} defaultSelectedKeys={["iceland"]} />
    ),
};

// Tags — multi-select union scope: checkboxes in the selection slot.
const tags: TreeNodeData[] = [
    {
        id: "places",
        icon: "tag",
        count: 982,
        label: "Places",
        children: [
            { id: "iceland", icon: "tag", count: 88, label: "Iceland" },
            {
                id: "us",
                icon: "tag",
                count: 640,
                label: "United States",
                children: [
                    { id: "portland", icon: "tag", count: 210, label: "Portland" },
                    { id: "seattle", icon: "tag", count: 430, label: "Seattle" },
                ],
            },
        ],
    },
    { id: "film", icon: "tag", count: 53, label: "Shot on film" },
];

export const Tags: Story = {
    render: () => (
        <Tree
            aria-label="Tags"
            nodes={tags}
            selectionMode="multiple"
            defaultExpandedKeys={["places", "us"]}
            defaultSelectedKeys={["iceland", "portland"]}
        />
    ),
};

// Collections — the same component, collection icons.
const collections: TreeNodeData[] = [
    {
        id: "portfolio",
        icon: "collection",
        count: 64,
        label: "Portfolio",
        children: [
            { id: "best", icon: "collection", count: 24, label: "Best of 2024" },
            { id: "print", icon: "collection", count: 40, label: "Print candidates" },
        ],
    },
    { id: "clients", icon: "collection", count: 318, label: "Clients" },
];

export const Collections: Story = {
    render: () => (
        <Tree aria-label="Collections" nodes={collections} defaultExpandedKeys={["portfolio"]} defaultSelectedKeys={["best"]} />
    ),
};
