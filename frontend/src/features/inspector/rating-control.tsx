import { Star } from "lucide-react";
import s from "./rating-control.module.css";

/** Five stars; click the current rating to clear. Emits 0 for "no rating". */
export const RatingControl = ({ value, onChange }: { value: number; onChange: (rating: number) => void }) => (
    <div className={s.stars}>
        {[1, 2, 3, 4, 5].map((n) => (
            <button key={n} className={s.star} aria-label={`${n}`} onClick={() => onChange(value === n ? 0 : n)}>
                <Star size={16} strokeWidth={1.5} fill={n <= value ? "var(--accent)" : "none"} color={n <= value ? "var(--accent)" : "var(--text-tertiary)"} />
            </button>
        ))}
    </div>
);
