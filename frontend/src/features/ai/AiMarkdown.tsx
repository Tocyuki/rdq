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
 * blocks and GFM extensions (tables, task lists). The highlight.js
 * stylesheet is imported once so every AI dialog picks up the same look.
 */
export function AiMarkdown({ markdown, className }: Props) {
  return (
    <div className={cn('prose prose-invert max-w-none text-sm [&_pre]:bg-[#0d1117] [&_pre]:p-3 [&_pre]:rounded-md [&_code]:font-mono', className)}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
        {markdown}
      </ReactMarkdown>
    </div>
  )
}
