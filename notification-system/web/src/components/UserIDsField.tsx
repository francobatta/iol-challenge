import { Field } from "@/components/common"
import { Textarea } from "@/components/ui/textarea"
import { maxUserIDs } from "@/lib/api"
import { parseIDs } from "@/lib/ids"

/**
 * A box to type or paste user IDs into, however they are separated. The caller keeps the
 * text and reads the IDs out of it with parseIDs.
 */
export function UserIDsField(props: { id: string; label: string; value: string; onChange: (value: string) => void }) {
  const count = parseIDs(props.value).length
  return (
    <Field
      label={props.label}
      htmlFor={props.id}
      hint={
        count > maxUserIDs ? (
          <span className="text-destructive">
            {count} users. At most {maxUserIDs} fit in one request; use a list for more.
          </span>
        ) : (
          `Separate IDs with commas, spaces or new lines. ${count} ${count === 1 ? "user" : "users"} so far.`
        )
      }
    >
      <Textarea
        id={props.id}
        rows={3}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
        placeholder="ana, bob, carla"
        spellCheck={false}
      />
    </Field>
  )
}
