# Magic sentences

A **magic sentence** makes a want from a short, fixed phrase. The want type `transit_search` has

```text
<alias> <from:station> <to:station>
```

and its aliases hold `transit`, so

::: v-pre
```sh
$ mywant m transit 荻窪 中野坂上
「荻窪→中野坂上」を作りました。
荻窪→中野坂上 (transit_search, want-c8ad…)
  departure: 16:49
  arrival: 16:59
  fare: 178
  legs_text: 荻窪(16:49) ─[東京メトロ丸ノ内線]→ 中野坂上(16:59)
```
:::

places the route — or, when a want with the same parameters is already there, shows that one. The same sentence works in the robot's chat and the MyWant keyboard. See [mywant magic](/reference/magic) for every flag.

## Take out what you need

```sh
mywant m transit 荻窪 中野坂上 --field departure        # 16:49, bare
mywant m transit 荻窪 中野坂上 --field departure,fare
mywant m transit 荻窪 中野坂上 --json
mywant wants restart $(mywant m transit 荻窪 中野坂上 --id)   # act on the want
```

## Writing one

Two labels on a want type (or on one want, which then wins over its type's):

| Label | What it holds |
| --- | --- |
| `magic-sentence` | The phrases, separated by `\|` or new lines |
| `aliases` | The type's other names, comma-separated — what `<alias>` stands for |

::: v-pre
| In a phrase | Means |
| --- | --- |
| `<from>` | The words there go into the parameter `from` |
| `<from:station>` | The same, preferring `station` things; completion offers only those |
| `<alias>` | Any of the aliases, case ignored; fills no parameter |
| a space | One or more spaces |
| anything else | Matched as written (`から`, `まで`) |
:::

```yaml
wantType:
  metadata:
    name: transit_search
    labels:
      aliases: "乗り換え,乗換,行き方,transit"
      magic-sentence: "<from:station>から<to:station>|<alias> <from:station> <to:station>"
```

Completion and the keyboards use a phrase that starts with `<alias>` and has its parts separated by spaces.
