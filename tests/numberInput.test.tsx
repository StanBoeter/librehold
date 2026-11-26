// @vitest-environment happy-dom
import { describe, it, expect, afterEach } from "vitest";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { NumberInput } from "../src/shared/NumberInput";

afterEach(() => cleanup());

function Harness({
  start,
  min,
  max,
  step,
  seen,
}: {
  start: number;
  min?: number;
  max?: number;
  step?: number;
  seen: number[];
}) {
  const [value, setValue] = useState(start);
  return (
    <>
      <NumberInput
        aria-label="n"
        value={value}
        min={min}
        max={max}
        step={step}
        onValue={(n) => {
          seen.push(n);
          setValue(n);
        }}
      />
      <button onClick={() => setValue(7)}>seven</button>
    </>
  );
}

const field = () => screen.getByLabelText("n") as HTMLInputElement;
const type = (text: string) =>
  fireEvent.change(field(), { target: { value: text } });

describe("number input", () => {
  it("can be emptied and typed into without snapping back", () => {
    const seen: number[] = [];
    render(<Harness start={0} min={1} max={40} seen={seen} />);
    fireEvent.focus(field());
    type("");
    expect(field().value).toBe("");
    type("2");
    type("25");
    expect(field().value).toBe("25");
    expect(seen).toEqual([2, 25]);
  });

  it("keeps what was typed out of range until the field is left", () => {
    const seen: number[] = [];
    render(<Harness start={19} min={1} max={40} seen={seen} />);
    fireEvent.focus(field());
    type("5");
    type("50");
    expect(field().value).toBe("50");
    expect(seen).toEqual([5, 40]);
    fireEvent.blur(field());
    expect(field().value).toBe("40");
  });

  it("puts the lowest number it may hold when left empty", () => {
    const seen: number[] = [];
    render(<Harness start={19} seen={seen} />);
    fireEvent.focus(field());
    type("");
    fireEvent.blur(field());
    // A field with no floor means 0: a player who deletes a number means to be rid of it, not to have it restored.
    expect(field().value).toBe("0");
    expect(seen).toEqual([0]);
  });

  it("puts the floor back when a field may not hold 0", () => {
    const seen: number[] = [];
    render(<Harness start={19} min={1} seen={seen} />);
    fireEvent.focus(field());
    type("");
    fireEvent.blur(field());
    // A level or a seat count has no meaning at 0, so an empty field means the smallest it may be.
    expect(field().value).toBe("1");
    expect(seen).toEqual([1]);
  });

  it("does not report a value it already holds", () => {
    const seen: number[] = [];
    render(<Harness start={0} seen={seen} />);
    fireEvent.focus(field());
    type("");
    fireEvent.blur(field());
    expect(field().value).toBe("0");
    expect(seen).toEqual([]);
  });

  it("clears a filled-in army rather than putting it back", () => {
    // The case that prompted this: "fill in my troops" puts 4000 in, and deleting it left 4000 again.
    const seen: number[] = [];
    render(<Harness start={4000} min={0} seen={seen} />);
    fireEvent.focus(field());
    type("");
    fireEvent.blur(field());
    expect(field().value).toBe("0");
    expect(seen).toEqual([0]);
  });

  it("rounds counts down and keeps fractions where the step allows them", () => {
    const counts: number[] = [];
    render(<Harness start={1} seen={counts} />);
    fireEvent.focus(field());
    type("3.7");
    expect(counts).toEqual([3]);
    cleanup();
    const speeds: number[] = [];
    render(<Harness start={1} step={0.5} seen={speeds} />);
    fireEvent.focus(field());
    type("1.5");
    expect(speeds).toEqual([1.5]);
  });

  it("shows a value set from outside once the field is left", () => {
    render(<Harness start={1} seen={[]} />);
    fireEvent.focus(field());
    type("");
    fireEvent.blur(field());
    fireEvent.click(screen.getByText("seven"));
    expect(field().value).toBe("7");
  });

  it("is the only number field in the client", () => {
    const files = (dir: string): string[] =>
      readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
        e.isDirectory()
          ? files(join(dir, e.name))
          : e.name.endsWith(".tsx")
            ? [join(dir, e.name)]
            : [],
      );
    const raw = files(join(__dirname, "../src")).filter(
      (f) =>
        !f.endsWith(join("shared", "NumberInput.tsx")) &&
        readFileSync(f, "utf8").includes('type="number"'),
    );
    expect(raw).toEqual([]);
  });
});
