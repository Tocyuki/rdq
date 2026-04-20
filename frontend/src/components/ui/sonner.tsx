import { Toaster as Sonner, type ToasterProps } from 'sonner'

/**
 * Toaster is rdq's standard toast host. bottom-right keeps the row of
 * header actions (Ask / Review / Analyze / Explain / Run, plus the
 * Change button on ConnectionBar) unobstructed — a toast that sits
 * on top of those buttons blocks further interaction until it fades.
 * richColors makes errors visually distinct from info toasts.
 */
export function Toaster(props: ToasterProps) {
  return <Sonner richColors position="bottom-right" {...props} />
}
