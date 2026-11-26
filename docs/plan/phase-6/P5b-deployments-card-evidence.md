# Put the acceptance evidence on the #deployments card

State: open

Source: design section 9 ("#deployments"): the card holds the tracker issue, preview URL, PR, video and screenshots, metrics, the blast radius level with reasons and the `trace_id`. Found on 2026-11-27: the card built in `askHuman` and the "ready" publication carry no preview URL and no acceptance artefacts.

What to do: after P5a, add the preview endpoint and links to the screenshots and video the acceptance job left in its job directory (stored as artefacts, section 14) to the card; metrics are the acceptance job's duration and cost. No file of the hidden scenarios goes on the card.

Check: a golden card in a T4 scenario after P5a's passing acceptance.
