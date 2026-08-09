/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import ReactMarkdown, { defaultUrlTransform } from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize from 'rehype-sanitize'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

interface MarkdownProps {
  breaks?: boolean
  children: string
  className?: string
}

export function Markdown({
  breaks = false,
  children,
  className,
}: MarkdownProps) {
  const remarkPlugins = breaks ? [remarkGfm, remarkBreaks] : [remarkGfm]

  return (
    <div
      className={cn(
        'prose prose-sm dark:prose-invert max-w-none',
        '[&_h1]:mt-6 [&_h1]:mb-3 [&_h1]:text-2xl [&_h1]:font-semibold',
        '[&_h2]:mt-5 [&_h2]:mb-3 [&_h2]:text-xl [&_h2]:font-semibold',
        '[&_h3]:mt-4 [&_h3]:mb-2 [&_h3]:text-lg [&_h3]:font-semibold',
        '[&_h4]:mt-4 [&_h4]:mb-2 [&_h4]:font-semibold',
        '[&_em]:italic [&_p]:my-2 [&_p]:leading-relaxed [&_strong]:font-semibold',
        '[&_a]:text-primary hover:[&_a]:text-primary/80 [&_a]:underline',
        '[&_li]:my-1 [&_li]:pl-1 [&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-5',
        '[&_blockquote]:border-primary [&_blockquote]:bg-muted/50 [&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:py-1 [&_blockquote]:pl-4',
        '[&_code]:bg-muted [&_code]:rounded [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono',
        '[&_pre]:bg-muted [&_pre]:my-3 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:border [&_pre]:p-3 [&_table]:my-4 [&_table]:block [&_table]:w-full [&_table]:overflow-x-auto',
        '[&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_pre_code]:text-sm',
        '[&_thead]:bg-muted [&_td]:border [&_td]:px-3 [&_td]:py-2 [&_th]:border [&_th]:px-3 [&_th]:py-2 [&_th]:text-left',
        '[&_hr]:my-6 [&_img]:my-4 [&_img]:max-w-full [&_img]:rounded-lg',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        '[overflow-wrap:anywhere]',
        className
      )}
    >
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={[rehypeRaw, rehypeSanitize]}
        urlTransform={defaultUrlTransform}
        components={{
          // 所有外链统一在新窗口打开，并阻断 opener 引用。
          a: ({ node, ...props }) => (
            <a {...props} target='_blank' rel='noopener noreferrer' />
          ),
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
