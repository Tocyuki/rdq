import ReactMarkdown from 'react-markdown'
import rehypeHighlight from 'rehype-highlight'
import remarkGfm from 'remark-gfm'
import 'highlight.js/styles/github-dark.css'

import { cn } from '@/lib/utils'

interface Props {
  markdown: string
  className?: string
}

/**
 * AiMarkdown renders a Bedrock response with syntax highlighting on code
 * blocks and GFM extensions (tables, task lists).
 *
 * Typography is provided by @tailwindcss/typography (loaded via @plugin
 * in index.css). `prose-sm` keeps the compact sizing consistent with
 * dialog bodies and a few prose-* overrides tighten heading / list
 * spacing so the result fills the scroll area without feeling like a
 * blog post. `dark:prose-invert` flips colours only in dark mode — on
 * the default light dialog background the text must stay dark or it
 * becomes invisible.
 *
 * The fenced-code `<pre>` keeps a dark (github-dark) background in both
 * themes: highlight.js emits tokens coloured for a dark canvas, so a
 * light pre background would render yellow/orange keywords nearly
 * invisible.
 */
export function AiMarkdown({ markdown, className }: Props) {
  return (
    <div
      className={cn(
        'prose prose-sm dark:prose-invert max-w-none',
        'prose-headings:mt-4 prose-headings:mb-2 prose-headings:font-semibold',
        'prose-p:my-2 prose-li:my-0 prose-ul:my-2 prose-ol:my-2',
        'prose-code:font-mono prose-code:text-[13px]',
        'prose-code:before:content-none prose-code:after:content-none',
        'prose-pre:bg-[#0d1117] prose-pre:text-slate-100 prose-pre:border prose-pre:border-border prose-pre:rounded-md prose-pre:p-3',
        'prose-a:text-primary prose-a:underline-offset-2',
        'prose-table:text-xs prose-th:text-left',
        className,
      )}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
        {markdown}
      </ReactMarkdown>
    </div>
  )
}
