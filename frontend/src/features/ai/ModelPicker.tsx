import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useSession } from '@/hooks/useSession'

import { useAiModels } from './useAiModels'

interface Props {
  value: string
  onChange: (id: string) => void
}

/**
 * ModelPicker is a standalone Select bound to /api/ai/models. Empty /
 * loading states are rendered inline so dialogs can embed it without
 * their own loading skeletons.
 */
export function ModelPicker({ value, onChange }: Props) {
  const session = useSession()
  const models = useAiModels(session.data?.profile ?? '')
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-8 text-xs">
        <SelectValue placeholder={models.isLoading ? 'Loading…' : 'Pick a model'} />
      </SelectTrigger>
      <SelectContent>
        {models.data?.models.map((m) => (
          <SelectItem key={m.id} value={m.id}>
            {m.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
