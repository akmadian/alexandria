// The trigger renders closed; an open menu (defaultOpen) exposes its items with the
// disabled / destructive / selected hooks. Opening by click, hover-submenus, and keyboard
// nav are RAC overlay behavior — verified in the real browser (frontend/CLAUDE.md §8:
// portals/overlays are unreliable to drive in happy-dom).

import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import { Button } from "@/components/button/button";
import { Menu, MenuItem, MenuSeparator, MenuTrigger } from "./menu";

test("the trigger renders", () => {
    render(
        <MenuTrigger>
            <Button>Actions</Button>
            <Menu aria-label="Actions">
                <MenuItem id="rename">Rename</MenuItem>
            </Menu>
        </MenuTrigger>,
    );
    expect(screen.getByRole("button", { name: "Actions" })).toBeInTheDocument();
});

test("an open menu shows its items, with the disabled + destructive hooks", () => {
    render(
        <MenuTrigger defaultOpen>
            <Button>Actions</Button>
            <Menu aria-label="Actions">
                <MenuItem id="rename" icon="rating">
                    Rename
                </MenuItem>
                <MenuItem id="archive" isDisabled>
                    Archive
                </MenuItem>
                <MenuSeparator />
                <MenuItem id="delete" isDestructive>
                    Delete
                </MenuItem>
            </Menu>
        </MenuTrigger>,
    );
    expect(screen.getByRole("menuitem", { name: "Rename" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Archive" })).toHaveAttribute("data-disabled");
    expect(screen.getByRole("menuitem", { name: "Delete" })).toHaveAttribute("data-destructive");
});

test("a single-selection menu marks the selected item", () => {
    render(
        <MenuTrigger defaultOpen>
            <Button>View</Button>
            <Menu aria-label="View" selectionMode="single" defaultSelectedKeys={["loupe"]}>
                <MenuItem id="grid">Grid</MenuItem>
                <MenuItem id="loupe">Loupe</MenuItem>
            </Menu>
        </MenuTrigger>,
    );
    expect(screen.getByRole("menuitemradio", { name: "Loupe" })).toHaveAttribute("aria-checked", "true");
});

test("a multiple-selection menu marks every selected item", () => {
    render(
        <MenuTrigger defaultOpen>
            <Button>Columns</Button>
            <Menu aria-label="Columns" selectionMode="multiple" defaultSelectedKeys={["name", "rating"]}>
                <MenuItem id="name">File name</MenuItem>
                <MenuItem id="rating">Rating</MenuItem>
                <MenuItem id="date">Capture date</MenuItem>
            </Menu>
        </MenuTrigger>,
    );
    expect(screen.getByRole("menuitemcheckbox", { name: "File name" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("menuitemcheckbox", { name: "Rating" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("menuitemcheckbox", { name: "Capture date" })).toHaveAttribute("aria-checked", "false");
});
