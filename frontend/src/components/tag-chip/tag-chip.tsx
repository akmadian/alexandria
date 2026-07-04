import { X } from "lucide-react";
import { Button } from "@/components/button/button";
import { Icon } from "@/components/icon/icon";
import type { ColorLabel } from "@/api/contract";
import s from "./tag-chip.module.css";

// Tag colors are DATA colors (--label-*), identical across themes — the one
// place hue is allowed besides the accent.
interface TagChipProps {
    name: string;
    color?: ColorLabel | null;
    onRemove?: () => void;
}

export const TagChip = ({ name, color, onRemove }: TagChipProps) => (
    <span className={s.chip}>
        {color && <span className={s.dot} style={{ background: `var(--label-${color})` }} />}
        <span className={s.name}>{name}</span>
        {onRemove && (
            <Button size="sm" className={s.remove} onPress={onRemove} aria-label={`Remove ${name}`}>
                <Icon icon={X} size={12} />
            </Button>
        )}
    </span>
);
