// The tree renders inline (docked chrome, not a portal), so happy-dom drives it. RAC Tree is
// treegrid-based: items are role="row" with aria-level / aria-expanded / aria-selected. The count
// joins a row's accessible name, so names are matched by regex. computeGuides is pure and gets its
// own unit coverage — it's the connector model the whole visual depends on.

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { formatCount, formatNumber } from "@/lib/format";
import { computeGuides, Tree, type TreeNodeData } from "./tree";

const folders: TreeNodeData[] = [
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
    { id: "misc", icon: "folder", count: 3, label: "Misc" },
];

function Folders(props: { selectionMode?: "single" | "multiple"; selectedKeys?: string[] }) {
    return (
        <Tree
            aria-label="Folders"
            nodes={folders}
            defaultExpandedKeys={["2024"]}
            selectionMode={props.selectionMode}
            selectedKeys={props.selectedKeys}
        />
    );
}

// ── computeGuides (the connector model) ──────────────────────────────────────

test("computeGuides: top-level nodes have no connector", () => {
    const guides = computeGuides(folders);
    expect(guides.get("2024")).toEqual([]);
    expect(guides.get("misc")).toEqual([]);
});

test("computeGuides: a child's own column is tee (has sibling) or end (last child)", () => {
    const guides = computeGuides(folders);
    expect(guides.get("iceland")).toEqual(["tee"]); // Japan follows
    expect(guides.get("japan")).toEqual(["end"]); // last child of 2024
});

test("computeGuides: a deeper node draws ancestor verticals (line) then its own elbow", () => {
    const nested: TreeNodeData[] = [
        {
            id: "a",
            label: "a",
            children: [
                { id: "a1", label: "a1", children: [{ id: "a1x", label: "a1x" }, { id: "a1y", label: "a1y" }] },
                { id: "a2", label: "a2" },
            ],
        },
    ];
    const guides = computeGuides(nested);
    // a1 still has a2 after it → its subtree draws a pass-through "line"; a1x/a1y are its children.
    expect(guides.get("a1x")).toEqual(["line", "tee"]); // a1 continues, a1y follows
    expect(guides.get("a1y")).toEqual(["line", "end"]); // a1 continues, a1y is last
    // a2 is the last child of a → its subtree (none) would carry "blank", and a2's own column is "end".
    expect(guides.get("a2")).toEqual(["end"]);
});

// ── Behavior (items-driven RAC render) ───────────────────────────────────────

test("renders nested nodes; expanded parents reveal their children", () => {
    render(<Folders />);
    expect(screen.getByRole("row", { name: /2024/ })).toHaveAttribute("aria-level", "1");
    expect(screen.getByRole("row", { name: /Iceland/ })).toHaveAttribute("aria-level", "2");
});

test("a parent exposes expand state; a leaf does not", () => {
    render(<Folders />);
    expect(screen.getByRole("row", { name: /2024/ })).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("row", { name: /Iceland/ })).not.toHaveAttribute("aria-expanded");
});

test("count is §13-formatted: exact ≤4 digits, abbreviated beyond", () => {
    expect(formatCount(9999)).toBe(formatNumber(9999));
    expect(formatCount(10000)).not.toBe(formatNumber(10000));
    render(<Folders />);
    expect(screen.getByText(formatCount(412))).toBeInTheDocument();
    expect(screen.getByText(formatCount(20300))).toBeInTheDocument();
});

test("single-select marks the one scope node (no checkboxes)", () => {
    render(<Folders selectionMode="single" selectedKeys={["iceland"]} />);
    expect(screen.getByRole("row", { name: /Iceland/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});

test("multiple-select marks every selected node and renders checkboxes", () => {
    render(<Folders selectionMode="multiple" selectedKeys={["iceland", "misc"]} />);
    expect(screen.getByRole("row", { name: /Iceland/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("row", { name: /Misc/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("row", { name: /2024/ })).toHaveAttribute("aria-selected", "false");
    expect(screen.getAllByRole("checkbox").length).toBeGreaterThan(0);
});
