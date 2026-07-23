import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import { Kbd, KbdGroup } from "@/components/kbd/kbd";
import {
    Menu,
    MenuItem,
    MenuSection,
    MenuSectionHeader,
    MenuSeparator,
    MenuTrigger,
    SubmenuTrigger,
} from "./menu";

// Menu — the §6 transient's second tenant (a role=menu roving list on the shared Popover).
// Stories mirror the design-library specimens; theme is switched from the toolbar.
const meta = {
    title: "Primitives/Menu",
    component: Menu,
    // Menu is composition-first (items are children); each story's render supplies the real
    // subtree, so this placeholder only satisfies the required children prop.
    args: { children: null },
} satisfies Meta<typeof Menu>;

export default meta;

type Story = StoryObj<typeof meta>;

// A labeled dropdown trigger: a ghost button (a button, not a field — distinct from Select),
// plus the `disclose` chevron rotated to point down.
function DropdownTrigger({ children }: { children: ReactNode }) {
    return (
        <Button rung="ghost">
            {children}
            <span style={{ display: "inline-flex", color: "var(--alx-ink-3)", transform: "rotate(90deg)" }}>
                <Icon concept="disclose" />
            </span>
        </Button>
    );
}

const row = { display: "flex", flexWrap: "wrap", alignItems: "flex-start", gap: "var(--alx-space-3)" } as const;
const caption = { margin: "0 0 var(--alx-space-3)", maxWidth: "40ch" } as const;

// The flagship action menu — icons, shortcuts, a section, a two-line description, a disabled
// row, a destructive row, and a hover-open submenu. `open` renders it statically for review.
function ActionMenu({ open }: { open?: boolean }) {
    return (
        <MenuTrigger defaultOpen={open}>
            <DropdownTrigger>Actions</DropdownTrigger>
            <Menu aria-label="Asset actions">
                <SubmenuTrigger>
                    <MenuItem id="rate" icon="rating">
                        Rate
                    </MenuItem>
                    <Menu aria-label="Rate">
                        <MenuItem id="r0">None</MenuItem>
                        <MenuItem id="r1">1 star</MenuItem>
                        <MenuItem id="r2">2 stars</MenuItem>
                        <MenuItem id="r3">3 stars</MenuItem>
                        <MenuItem id="r4">4 stars</MenuItem>
                        <MenuItem id="r5">5 stars</MenuItem>
                    </Menu>
                </SubmenuTrigger>
                <MenuItem id="pick" icon="flag" shortcut="P">
                    Flag as pick
                </MenuItem>
                <MenuItem id="reject" icon="reject" shortcut="X">
                    Reject
                </MenuItem>
                <MenuSeparator />
                <MenuSection>
                    <MenuSectionHeader>More</MenuSectionHeader>
                    <MenuItem id="meta" icon="settings">
                        Metadata settings…
                    </MenuItem>
                    <MenuItem id="hide" description="Stays in the catalog; hidden from this view.">
                        Hide from grid
                    </MenuItem>
                    <MenuItem id="edit" isDisabled>
                        Open in external editor
                    </MenuItem>
                </MenuSection>
                <MenuSeparator />
                <MenuItem
                    id="remove"
                    isDestructive
                    shortcut={
                        <KbdGroup>
                            <Kbd icon="command" />
                            <Kbd icon="delete" />
                        </KbdGroup>
                    }
                >
                    Remove from catalog…
                </MenuItem>
            </Menu>
        </MenuTrigger>
    );
}

// Interactive: click the trigger to open. The real usage — a ghost button revealing a menu.
export const Playground: Story = {
    render: () => <ActionMenu />,
};

// The same action menu rendered open, so the whole item anatomy is reviewable at a glance.
export const Open: Story = {
    render: () => <ActionMenu open />,
};

// Selection: single (a view switcher, radio) and multiple (a column picker, checkboxes). Each
// selected row carries the hue-free check; open each to see it.
export const Selection: Story = {
    render: () => (
        <div style={row}>
            <MenuTrigger>
                <DropdownTrigger>View</DropdownTrigger>
                <Menu aria-label="View mode" selectionMode="single" defaultSelectedKeys={["loupe"]}>
                    <MenuItem id="grid">Grid</MenuItem>
                    <MenuItem id="loupe">Loupe</MenuItem>
                    <MenuItem id="compare">Compare</MenuItem>
                    <MenuItem id="cull">Cull</MenuItem>
                </Menu>
            </MenuTrigger>
            <MenuTrigger>
                <DropdownTrigger>Columns</DropdownTrigger>
                <Menu aria-label="Columns" selectionMode="multiple" defaultSelectedKeys={["name", "rating"]}>
                    <MenuItem id="name">File name</MenuItem>
                    <MenuItem id="rating">Rating</MenuItem>
                    <MenuItem id="date">Capture date</MenuItem>
                    <MenuItem id="camera">Camera</MenuItem>
                    <MenuItem id="lens">Lens</MenuItem>
                </Menu>
            </MenuTrigger>
        </div>
    ),
};

// Icon alignment is menu-scoped: any leading icon reserves the gutter for EVERY row (iconless
// rows indent to align); no icons collapses it (labels flush). Open both to compare.
export const Alignment: Story = {
    render: () => (
        <div>
            <p className="alx-type-caption" style={caption}>
                Left: some rows carry icons, so every row reserves the gutter. Right: no icons, so
                labels sit flush.
            </p>
            <div style={row}>
                <MenuTrigger>
                    <DropdownTrigger>Mixed icons</DropdownTrigger>
                    <Menu aria-label="Mixed icons">
                        <MenuItem id="space" icon="settings">
                            Space settings
                        </MenuItem>
                        <MenuItem id="fav" icon="flag">
                            Add to favorites
                        </MenuItem>
                        <MenuItem id="rename">Rename</MenuItem>
                        <MenuItem id="dupe">Duplicate</MenuItem>
                    </Menu>
                </MenuTrigger>
                <MenuTrigger>
                    <DropdownTrigger>No icons</DropdownTrigger>
                    <Menu aria-label="No icons">
                        <MenuItem id="rename2">Rename</MenuItem>
                        <MenuItem id="dupe2">Duplicate</MenuItem>
                        <MenuItem id="copy">Copy link</MenuItem>
                        <MenuItem id="arch">Archive</MenuItem>
                    </Menu>
                </MenuTrigger>
            </div>
        </div>
    ),
};
