import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Markdown } from "./Markdown";

describe("Markdown", () => {
  it("renders markdown structure", () => {
    const { container } = render(
      <Markdown
        text={"# Title\n\nSome **bold** text\n\n- one\n- two\n\n```ts\nconst a = 1;\n```"}
      />,
    );
    expect(screen.getByRole("heading", { level: 1, name: "Title" })).toBeInTheDocument();
    expect(screen.getByText("bold").tagName).toBe("STRONG");
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    const pre = container.querySelector("pre.markdown__pre");
    expect(pre).not.toBeNull();
    expect(pre?.textContent).toContain("const a = 1;");
  });

  it("shows raw HTML as text instead of executing it", () => {
    const { container } = render(<Markdown text={'<script>alert("x")</script>'} />);
    expect(container.querySelector("script")).toBeNull();
    expect(container.textContent).toContain('<script>alert("x")</script>');
  });

  it("replaces remote images with their description", () => {
    const { container } = render(
      <Markdown text={"![diagram](https://example.com/a.png)"} />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("[image: diagram]")).toBeInTheDocument();
  });

  it("opens links outside the application", () => {
    render(<Markdown text={"[docs](https://example.com)"} />);
    const link = screen.getByRole("link", { name: "docs" });
    expect(link).toHaveAttribute("href", "https://example.com");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer noopener");
  });

  it("wraps tables for horizontal scrolling", () => {
    const { container } = render(<Markdown text={"| a | b |\n| - | - |\n| 1 | 2 |"} />);
    expect(container.querySelector(".markdown__table table")).not.toBeNull();
  });
});
