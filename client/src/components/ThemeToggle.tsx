import { useTheme } from '../context/ThemeContext'
import { Sun, Moon } from 'lucide-react'
import { Button, Tooltip } from '@heroui/react'

export default function ThemeToggle() {
  const { theme, toggleTheme } = useTheme()

  return (
    <Tooltip>
      <Tooltip.Trigger>
        <Button
          isIconOnly
          variant="ghost"
          onPress={toggleTheme}
          aria-label={`切换至${theme === 'dark' ? '日间' : '夜间'}模式`}
          className="h-9 w-9 min-w-9 rounded-lg border border-border bg-surface text-muted shadow-none hover:bg-surface-secondary hover:text-foreground"
        >
          <div className="flex h-4 w-4 items-center justify-center">
            {theme === 'dark' ? (
              <Sun className="h-4 w-4 text-amber-400" />
            ) : (
              <Moon className="h-4 w-4" />
            )}
          </div>
        </Button>
      </Tooltip.Trigger>
      <Tooltip.Content className="bg-black/90 text-white px-2 py-1 rounded text-xs">
        切换至{theme === 'dark' ? '日间' : '夜间'}模式
      </Tooltip.Content>
    </Tooltip>
  )
}
