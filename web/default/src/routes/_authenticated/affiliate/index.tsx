import { createFileRoute } from '@tanstack/react-router'
import { Affiliate } from '@/features/affiliate'

export const Route = createFileRoute('/_authenticated/affiliate/')({
  component: Affiliate,
})
