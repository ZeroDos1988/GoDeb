// GoDeb logo: the shield (public/logo.png) and the "GoDeb" lettering
// (public/wordmark.png). Sources and sizes: docs/brand/README.md.

export function Shield({ className = "h-10 w-auto" }: { className?: string }) {
  return <img src="/logo.png" alt="" aria-hidden="true" className={`${className} shrink-0 select-none`} draggable={false} />;
}

export function Wordmark({ className = "h-5 w-auto" }: { className?: string }) {
  return <img src="/wordmark.png" alt="GoDeb" className={`${className} select-none`} draggable={false} />;
}
