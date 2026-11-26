import { useState, type InputHTMLAttributes } from "react";

type Props = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  "type" | "value" | "onChange" | "min" | "max" | "step"
> & {
  value: number;
  onValue: (n: number) => void;
  min?: number;
  max?: number;
  step?: number;
};

/**
 * A number field that shows exactly what the player types while it has focus. Parsing and clamping on every
 * keystroke would turn an emptied field back into its minimum at once, so only a complete number is passed on,
 * clamped, and the text is tidied when the field is left.
 *
 * An emptied field means the lowest number it may hold, which is 0 unless the field has a higher floor. It used to
 * keep the value it already had, so deleting a number and leaving the field silently put it back: a player who
 * cleared "4000" was left with 4000 again.
 */
export function NumberInput({
  value,
  onValue,
  min,
  max,
  step,
  onFocus,
  onBlur,
  ...rest
}: Props) {
  const [draft, setDraft] = useState<string | null>(null);
  const whole = step === undefined || Number.isInteger(step);
  /** What an empty field means: the floor it may hold, which is 0 for a field with no floor. */
  const empty = min ?? 0;
  return (
    <input
      {...rest}
      type="number"
      inputMode={whole ? "numeric" : "decimal"}
      min={min}
      max={max}
      step={step}
      value={draft ?? value}
      onFocus={(e) => {
        setDraft(String(value));
        onFocus?.(e);
      }}
      onBlur={(e) => {
        // Read the draft before dropping it: it is the only record of a field the player left empty.
        const typed = draft;
        setDraft(null);
        if (typed !== null && typed.trim() === "" && value !== empty)
          onValue(empty);
        onBlur?.(e);
      }}
      onChange={(e) => {
        const text = e.target.value;
        if (draft !== null) setDraft(text);
        if (text.trim() === "") return;
        let n = Number(text);
        if (!Number.isFinite(n)) return;
        if (whole) n = Math.floor(n);
        if (min !== undefined) n = Math.max(min, n);
        if (max !== undefined) n = Math.min(max, n);
        onValue(n);
      }}
    />
  );
}
