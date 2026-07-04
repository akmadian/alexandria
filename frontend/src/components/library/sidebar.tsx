import {
    Bookmark,
    Clock,
    Folder,
    HardDrive,
    Image01,
    Star01,
    Tag01,
} from "@untitledui/icons";
import type { FC } from "react";
import { collections, colorLabelClass, sources, tags } from "@/api/mock";
import { cx } from "@/utils/cx";
import { StatusDot } from "./bits";

interface SidebarProps {
    active: string;
    onSelect: (key: string) => void;
    total: number;
}

const SectionLabel = ({ children }: { children: string }) => (
    <p className="px-2 pt-4 pb-1 text-xs font-semibold tracking-wide text-quaternary uppercase">{children}</p>
);

const Row = ({
    icon: Icon,
    label,
    count,
    active,
    onClick,
    trailing,
    dot,
}: {
    icon?: FC<{ className?: string }>;
    label: string;
    count?: number;
    active?: boolean;
    onClick: () => void;
    trailing?: React.ReactNode;
    dot?: string;
}) => (
    <button
        type="button"
        onClick={onClick}
        className={cx(
            "group flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition duration-100 ease-linear",
            active ? "bg-active font-medium text-secondary" : "text-tertiary hover:bg-primary_hover hover:text-secondary",
        )}
    >
        {Icon && <Icon className={cx("size-4 shrink-0", active ? "text-fg-brand-primary" : "text-fg-quaternary")} />}
        {dot && <span className={cx("size-2.5 shrink-0 rounded-full ring-1 ring-inset ring-black/10", dot)} />}
        {trailing}
        <span className="truncate">{label}</span>
        {count !== undefined && (
            <span className="ml-auto text-xs text-quaternary tabular-nums">{count.toLocaleString()}</span>
        )}
    </button>
);

export const Sidebar = ({ active, onSelect, total }: SidebarProps) => {
    return (
        <aside className="flex h-dvh w-60 shrink-0 flex-col border-r border-secondary bg-primary">
            <div className="flex items-center gap-2.5 px-4 py-3.5">
                <div className="flex size-8 items-center justify-center rounded-md bg-brand-solid text-white">
                    <Image01 className="size-4.5" />
                </div>
                <div className="leading-tight">
                    <p className="text-sm font-semibold text-primary">Alexandria</p>
                    <p className="text-xs text-quaternary">Digital Asset Manager</p>
                </div>
            </div>

            <nav className="flex-1 overflow-y-auto px-2 pb-4">
                <SectionLabel>Library</SectionLabel>
                <Row icon={Image01} label="All Assets" count={total} active={active === "all"} onClick={() => onSelect("all")} />
                <Row icon={Clock} label="Recent Imports" count={12} active={active === "recent"} onClick={() => onSelect("recent")} />
                <Row icon={Star01} label="Picks" count={6} active={active === "picks"} onClick={() => onSelect("picks")} />

                <SectionLabel>Sources</SectionLabel>
                {sources.map((s) => (
                    <Row
                        key={s.id}
                        icon={HardDrive}
                        label={s.name}
                        count={s.count}
                        active={active === `source:${s.id}`}
                        onClick={() => onSelect(`source:${s.id}`)}
                        trailing={<StatusDot status={s.status === "active" ? "online" : "offline"} />}
                    />
                ))}

                <SectionLabel>Collections</SectionLabel>
                {collections.map((c) => (
                    <Row
                        key={c.id}
                        icon={c.kind === "smart" ? Bookmark : Folder}
                        label={c.name}
                        count={c.count}
                        active={active === `collection:${c.id}`}
                        onClick={() => onSelect(`collection:${c.id}`)}
                    />
                ))}

                <SectionLabel>Tags</SectionLabel>
                {tags.map((t) => (
                    <Row
                        key={t.id}
                        dot={colorLabelClass[t.color]}
                        label={t.name}
                        count={t.count}
                        active={active === `tag:${t.id}`}
                        onClick={() => onSelect(`tag:${t.id}`)}
                    />
                ))}
            </nav>

            <div className="border-t border-secondary px-4 py-3">
                <div className="flex items-center gap-1.5 text-xs text-quaternary">
                    <Tag01 className="size-3.5" />
                    <span>24 assets · 2 sources online</span>
                </div>
            </div>
        </aside>
    );
};
