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
 * dialog bodies, `prose-invert` flips colours for the dark theme, and a
 * few prose-* overrides tighten heading / list spacing so the result
 * fills the scroll area without feeling like a blog post.
 *
 * `[&_pre]` + highlight.js `github-dark.css` style the fenced code
 * blocks; without the classes the pre tag would otherwise pick up the
 * prose-default off-white background and clash with the highlighted
 * tokens.
 */
export function AiMarkdown({ markdown, className }: Props) {
  return (
    <div
      className={cn(
        'prose prose-sm prose-invert max-w-none',
        'prose-headings:mt-4 prose-headings:mb-2 prose-headings:font-semibold',
        'prose-p:my-2 prose-li:my-0 prose-ul:my-2 prose-ol:my-2',
        'prose-code:font-mono prose-code:text-[13px]',
        'prose-code:before:content-none prose-code:after:content-none',
        'prose-pre:bg-[#0d1117] prose-pre:border prose-pre:border-border prose-pre:rounded-md prose-pre:p-3',
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
