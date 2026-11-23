// A confirmation stays tied to the session in which the user opened the dialog.
export function bindDialogSession(storage: Pick<Storage, 'getItem'>, close: () => void) {
  const confirmedSession = storage.getItem('token')
  const controller = new AbortController()
  return {
    options: { confirmedSession, signal: controller.signal },
    active() {
      if (!controller.signal.aborted && storage.getItem('token') !== confirmedSession) {
        controller.abort()
        close()
      }
      return !controller.signal.aborted
    },
    abort: () => controller.abort(),
  }
}
