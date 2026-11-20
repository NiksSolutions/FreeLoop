// newest guards a read the window may ask again before an earlier ask has answered,
// a list refreshed after each change, say: only the reply to the newest ask is used,
// so an older one arriving late does not put back what a newer one replaced (a
// project just trusted shown untrusted again), and an older one failing late says
// nothing. Each list has its own.
export function newest(): <T>(read: Promise<T>, use: (value: T) => void) => Promise<void> {
  let asked = 0;
  return <T>(read: Promise<T>, use: (value: T) => void) => {
    const mine = ++asked;
    return read.then(
      (value) => {
        if (mine === asked) use(value);
      },
      (err: unknown) => {
        if (mine === asked) throw err;
      },
    );
  };
}
