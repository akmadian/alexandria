// Left pane: the segmented mode selector (Sources | Collections | Tags) over
// one reusable Tree, plus the two fixed library shortcuts. Selecting anything
// dispatches a BrowseTarget; deriveListQuery does the rest.

import { Clock, LibraryBig } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { cx } from "@/lib/cx";
import { Button, Toggle } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import { Tree, type TreeNode } from "@/components/tree/tree";
import { useLibraryDispatch, useLibraryState, type BrowseTarget } from "@/app/library-state";
import { useCollections, useSources, useTags } from "@/api/queries";
import type { Collection, Source, Tag } from "@/api/contract";
import { collectionsToNodes, sourcesToNodes, tagsToNodes } from "./adapt";
import s from "./browser-view.module.css";

type Mode = "sources" | "collections" | "tags";
const MODES: Mode[] = ["sources", "collections", "tags"];

/** TreeNode ids are "kind:id"; targets carry them apart again. */
const nodeIdToTarget = (nodeId: string): BrowseTarget => {
    const [kind, id] = nodeId.split(":");
    return { kind: kind as "source" | "collection" | "tag", id };
};
const targetToNodeId = (t: BrowseTarget): string | null => ("id" in t ? `${t.kind}:${t.id}` : null);

export const BrowserView = () => {
    const { t } = useTranslation();
    const [mode, setMode] = useState<Mode>("collections");
    const state = useLibraryState();
    const dispatch = useLibraryDispatch();

    const sources = useSources().data ?? [];
    const collections = useCollections().data ?? [];
    const tags = useTags().data ?? [];

    const nodes: TreeNode<Source | Collection | Tag>[] =
        mode === "sources" ? sourcesToNodes(sources) : mode === "collections" ? collectionsToNodes(collections) : tagsToNodes(tags);
    const selectedId = targetToNodeId(state.target);

    return (
        <div className={s.browser}>
            <div className={s.shortcuts}>
                <Button className={cx(s.shortcut, state.target.kind === "all" && s.active)} onPress={() => dispatch({ type: "selectTarget", target: { kind: "all" } })}>
                    <Icon icon={LibraryBig} size={14} />
                    {t("browser.allAssets")}
                </Button>
                <Button
                    className={cx(s.shortcut, state.target.kind === "recent" && s.active)}
                    onPress={() => dispatch({ type: "selectTarget", target: { kind: "recent" } })}
                >
                    <Icon icon={Clock} size={14} />
                    {t("browser.recent")}
                </Button>
            </div>

            <div className={s.modes} role="tablist" aria-label={t("browser.mode.collections")}>
                {MODES.map((m) => (
                    <Toggle key={m} size="sm" className={s.mode} isSelected={mode === m} onChange={() => setMode(m)}>
                        {t(`browser.mode.${m}`)}
                    </Toggle>
                ))}
            </div>

            {nodes.length === 0 ? (
                <p className={s.empty}>{t("browser.empty")}</p>
            ) : (
                <Tree
                    treeId={mode}
                    aria-label={t(`browser.mode.${mode}`)}
                    nodes={nodes}
                    selectedId={selectedId}
                    onSelect={(node) => dispatch({ type: "selectTarget", target: nodeIdToTarget(node.id) })}
                    renderDecoration={(node) => (
                        <>
                            {"status" in node.data && node.data.status === "offline" && <span className={s.offlineDot} title={t("sourceStatus.offline")} />}
                            {"color" in node.data && <span className={s.colorDot} style={{ background: `var(--label-${node.data.color})` }} />}
                            <span className="u-data">{node.data.count}</span>
                        </>
                    )}
                />
            )}
        </div>
    );
};
