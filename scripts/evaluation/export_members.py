"""Export target member indices used by the CIFAR-10 demo configuration."""

import numpy as np
import json

train_size = 3000
remain_size = 46000
test_size = 1000
lengths = [train_size, remain_size, test_size]

indices = list(range(sum(lengths)))

np.random.seed(1)
np.random.shuffle(indices)

# The target model uses the first 3000 shuffled CIFAR-10 training samples.
true_members = indices[:50]

with open("target_members.json", "w") as f:
    json.dump(true_members, f)

print("Exported 50 target member indices to target_members.json")
print("First 5 member indices:", true_members[:5])
