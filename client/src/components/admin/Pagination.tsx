import { Button } from '@heroui/react'
import { ChevronLeft, ChevronRight } from 'lucide-react'

interface Props {
  page: number
  total: number
  pageSize: number
  onChange: (page: number) => void
}

export default function Pagination({ page, total, pageSize, onChange }: Props) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  if (totalPages <= 1) return null

  return (
    <div className="flex items-center justify-between mt-6 pt-4 border-t border-border">
      <span className="text-xs text-muted">
        第 {page} / {totalPages} 页，共 {total} 条
      </span>
      <div className="flex items-center gap-1">
        <Button
          size="sm"
          variant="ghost"
          isDisabled={page <= 1}
          onPress={() => onChange(page - 1)}
          className="px-2 py-1 h-8 min-w-0 cursor-pointer text-muted hover:text-foreground"
        >
          <ChevronLeft className="w-4 h-4" />
        </Button>
        {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
          let pageNum: number
          if (totalPages <= 5) pageNum = i + 1
          else if (page <= 3) pageNum = i + 1
          else if (page >= totalPages - 2) pageNum = totalPages - 4 + i
          else pageNum = page - 2 + i
          return (
            <Button
              key={pageNum}
              size="sm"
              variant={page === pageNum ? 'primary' : 'ghost'}
              onPress={() => onChange(pageNum)}
              className={`px-3 py-1 h-8 min-w-0 text-xs cursor-pointer ${
                page === pageNum ? 'bg-emerald-500 text-white font-semibold' : 'text-muted hover:text-foreground'
              }`}
            >
              {pageNum}
            </Button>
          )
        })}
        <Button
          size="sm"
          variant="ghost"
          isDisabled={page >= totalPages}
          onPress={() => onChange(page + 1)}
          className="px-2 py-1 h-8 min-w-0 cursor-pointer text-muted hover:text-foreground"
        >
          <ChevronRight className="w-4 h-4" />
        </Button>
      </div>
    </div>
  )
}
