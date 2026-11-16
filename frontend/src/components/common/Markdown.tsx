import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

// Markdown for agent prose. react-markdown renders to React elements and never
// injects HTML, so raw HTML in harness output stays visible text and no
// sanitizer is needed. Remote images are replaced by their description because
// agent output must not beacon out of the local app, and links open outside the
// application window.
const components: Components = {
  a({ href, title, children }) {
    return (
      <a href={href} title={title} target="_blank" rel="noreferrer noopener">
        {children}
      </a>
    );
  },
  img({ alt }) {
    const label = alt?.trim();
    return (
      <span className="markdown__image" role="note">
        {label ? `[image: ${label}]` : "[image omitted]"}
      </span>
    );
  },
  pre({ children }) {
    return <pre className="code-block markdown__pre">{children}</pre>;
  },
  code({ className, children }) {
    return <code className={className ? `mono ${className}` : "mono"}>{children}</code>;
  },
  table({ children }) {
    return (
      <div className="markdown__table">
        <table>{children}</table>
      </div>
    );
  },
};

interface MarkdownProps {
  text: string;
  className?: string;
}

/** Renders markdown text; styling lives under the `.markdown` scope. */
export function Markdown({ text, className }: MarkdownProps) {
  return (
    <div className={className ? `markdown ${className}` : "markdown"}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {text}
      </ReactMarkdown>
    </div>
  );
}
