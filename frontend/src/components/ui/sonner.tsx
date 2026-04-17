import { Toaster as Sonner, type ToasterProps } from 'sonner'

/**
 * Toaster is rdq's standard toast host. We keep the default "top-right"
 * placement so the connection banner at the top of the screen does not
 * overlap toasts, and enable richColors so errors are immediately visually
 * distinct from informational toasts.
 */
export function Toaster(props: ToasterProps) {
  return <Sonner richColors position="top-right" {...props} />
}
