import os
import sys
import numpy as np
import pandas as pd
import matplotlib
matplotlib.use('Agg') # Run in headless mode to prevent blocking
import matplotlib.pyplot as plt
import json
import argparse
from scipy import stats
def parse_args():
    parser = argparse.ArgumentParser(description="LabelScan-Go Evaluation Script (Member B)")
    parser.add_argument("--ref", type=str, default="data/baseline_dists.json", help="Reference baseline stats output from Go")
    parser.add_argument("--test", type=str, default="data/audit_results.json", help="Audit results output from Go")
    parser.add_argument("--out", type=str, default="audit_report.csv", help="Output audit report path")
    parser.add_argument("--plot", type=str, default="risk_map.png", help="Output 2D risk map path")    return parser.parse_args()

def calculate_thresholds(reference_dists):
    """
    Step 3&4 in Method 2: Calculate Global Baseline (mu and std) from Reference distances (10 strangers).
    reference_dists: List of Float Arrays. Each array contains 11 L2 dists (1 original + 10 variants)
    """
    if len(reference_dists) == 0:
        return 0, 0
    
    d_bars = []
    cvs = []

    # Calculate intra-sample metrics.
    for dists in reference_dists:
        if len(dists) == 0:
            continue
        mean_d = np.mean(dists)
        std_d = np.std(dists, ddof=1) if len(dists) > 1 else 0
        d_bars.append(mean_d)
        
        cv = std_d / mean_d if mean_d > 0 else 0
        cvs.append(cv)
    
    # Calculate inter-sample metrics.
    mu_d = np.mean(d_bars)
    s_d = np.std(d_bars, ddof=1) if len(d_bars) > 1 else 0

    mu_cv = np.mean(cvs)
    s_cv = np.std(cvs, ddof=1) if len(cvs) > 1 else 0

    # Calculate the t-distribution coefficient for 95% confidence interval
    # n = 10 (freedom degrees = 9), alpha = 0.05
    n = len(reference_dists)
    if n > 1:
        t_val = stats.t.ppf(0.95, n - 1)
        coeff = t_val * np.sqrt(1 + 1.0 / n)
    else:
        coeff = 1.92 # fallback to the n=10 mathematical predefined 1.92
    
    tau_d = mu_d + coeff * s_d
    tau_cv = mu_cv - coeff * s_cv

    return tau_d, tau_cv, mu_d, mu_cv

def evaluate_risk(dbar, cv, tau_d, tau_cv, mu_d, mu_cv):
    """
    Calculate the Membership Risk Index (MRI) into a 0-100 score.
    And make red/yellow/green flag judgement.
    """
    # Is Distance Outlier?
    is_d_outlier = dbar > tau_d
    # Is CV Outlier?
    is_cv_outlier = cv < tau_cv

    # Multi-dimensional Judgement
    if is_d_outlier and is_cv_outlier:
        flag = "🔴 Confirmed (Member)"
        base_score = 80
    elif is_d_outlier or is_cv_outlier:
        flag = "🟡 Probable (High Risk)"
        base_score = 50
    else:
        flag = "🟢 Safe (Non-Member)"
        base_score = 10
    
    # Calculate fine-grained score
    d_ratio = dbar / mu_d if mu_d > 0 else 1
    cv_ratio = cv / mu_cv if cv > 0 else 1
    
    # Simple Weighting Formula
    # Distance ratio pushes score up, while lower CV ratio pushes score up.
    score_penalty_d = min(30, max(0, (d_ratio - 1) * 10))
    score_penalty_cv = min(30, max(0, (1 - cv_ratio) * 10))

    final_score = base_score + (score_penalty_d + score_penalty_cv) / 2
    final_score = min(100.0, final_score)
    
    return final_score, flag

def main():
    args = parse_args()
    
    # Placeholder: Handle JSON I/O loading
    # In a full-stream implementation, C (Acceleration Engineer) will dump JSON files for B to read.
    # Below is a simulation data block to make eval.py fully testable right now.
    
    print("🚀 [Member B] LabelScan-Go Evaluation Node starting...")
    print("-----------------------------------------------------")
    
    # Simulated Load
    dummy_reference_dists = [
        [0.85, 0.81, 0.88, 0.82, 0.80, 0.86, 0.85, 0.81, 0.88, 0.87, 0.84],
        [1.10, 1.15, 1.12, 1.18, 1.05, 1.08, 1.11, 1.16, 1.14, 1.09, 1.13],
        [0.95, 0.90, 0.98, 0.92, 0.89, 0.95, 0.94, 0.99, 0.97, 0.91, 0.96],
        [1.20, 1.25, 1.22, 1.28, 1.15, 1.18, 1.21, 1.26, 1.24, 1.19, 1.23],
        [0.75, 0.70, 0.78, 0.72, 0.69, 0.75, 0.74, 0.79, 0.77, 0.71, 0.76],
        [1.05, 1.00, 1.08, 1.02, 0.99, 1.05, 1.04, 1.09, 1.07, 1.01, 1.06],
        [1.30, 1.35, 1.32, 1.38, 1.25, 1.28, 1.31, 1.36, 1.34, 1.29, 1.33],
        [0.90, 0.85, 0.93, 0.87, 0.84, 0.90, 0.89, 0.94, 0.92, 0.86, 0.91],
        [1.15, 1.10, 1.18, 1.12, 1.09, 1.15, 1.14, 1.19, 1.17, 1.11, 1.16],
        [1.00, 0.95, 1.03, 0.97, 0.94, 1.00, 0.99, 1.04, 1.02, 0.96, 1.01]
    ]

    dummy_audit_dists = [
        {"id": 1, "dists": [3.20, 3.21, 3.19, 3.20, 3.22, 3.18, 3.20, 3.21, 3.19, 3.20, 3.22], "label_true": 0, "label_pred": 0}, # Member (far and very stable)
        {"id": 2, "dists": [1.02, 1.05, 0.88, 1.15, 0.90, 1.10, 0.95, 1.08, 0.85, 1.12, 0.92], "label_true": 1, "label_pred": 1}, # Non-Member (close and unstable)
        {"id": 3, "dists": [1.50, 1.48, 1.52, 1.49, 1.51, 1.50, 1.47, 1.53, 1.48, 1.52, 1.49], "label_true": 2, "label_pred": 2}  # Probable (semi-far, very stable)
    ]

    # 1. Calibrate System
    tau_d, tau_cv, mu_d, mu_cv = calculate_thresholds(dummy_reference_dists)
    
    print(f"📊 Calibration Complete (n=10):")
    print(f"  > Reference Dist Mean (mu_d):  {mu_d:.4f}")
    print(f"  > Reference CV Mean (mu_cv):   {mu_cv:.4f}")
    print(f"  > Dist Watermark Limit (tau_d): {tau_d:.4f} (Above is anomalous)")
    print(f"  > CV Watermark Limit (tau_cv):  {tau_cv:.4f} (Below is anomalous)")
    print("-" * 40)

    # 2. Evaluate Target Samples
    results = []
    
    for sample in dummy_audit_dists:
        dists = sample["dists"]
        dbar = np.mean(dists)
        cv = np.std(dists, ddof=1) / dbar if dbar > 0 else 0
        
        score, flag = evaluate_risk(dbar, cv, tau_d, tau_cv, mu_d, mu_cv)
        
        # Determine specific triggers for reporting context
        trigger_d = "YES" if dbar > tau_d else "NO"
        trigger_cv = "YES" if cv < tau_cv else "NO"

        results.append({
            "Sample_ID": sample["id"],
            "Label": sample["label_true"],
            "Mean_Distance": float(f"{dbar:.4f}"),
            "Volatility_CV": float(f"{cv:.4f}"),
            "Dist_Outlier": trigger_d,
            "CV_Outlier": trigger_cv,
            "Risk_Score": float(f"{score:.2f}"),
            "Conclusion": flag
        })
        print(f"🔍 Sample {sample['id']} (Class {sample['label_true']}): Dist={dbar:.2f}, CV={cv:.4f} | {flag} (Score: {score:.1f})")

    # 3. Output CSV Report
    df = pd.DataFrame(results)
    df.to_csv(args.out, index=False)
    print("-----------------------------------------------------")
    print(f"📝 Audit report written to {args.out}")

    # 4. Generate 2D Risk Map (Distance vs CV Space)
    # plt.figure(figsize=(10, 6))
    # plt.style.use('seaborn-v0_8-darkgrid')
    
    # Plot backgrounds marking the quadrants based on Thresholds
    # plt.axvline(x=tau_d, color='r', linestyle='--', label=r'Distance Threshold ($\tau_d$)')
    # plt.axhline(y=tau_cv, color='b', linestyle='--', label=r'CV Threshold ($\tau_{cv}$)')

    # Add descriptive zones
    # plt.fill_between([tau_d, plt.xlim()[1] if df['Mean_Distance'].max() < tau_d * 2 else df['Mean_Distance'].max() * 1.5], 
    #                 0, tau_cv, color='red', alpha=0.1, label='Target Region (🔴 Member)')    
    
    # Plot the sample points
    # colors_map = {"🔴 Confirmed (Member)": "red", "🟡 Probable (High Risk)": "orange", "🟢 Safe (Non-Member)": "green"}
    # for idx, row in df.iterrows():
    #     plt.scatter(row['Mean_Distance'], row['Volatility_CV'], 
    #                 color=colors_map[row['Conclusion']], s=100, 
    #                 edgecolor='black', zorder=5)
    #     plt.text(row['Mean_Distance'] + 0.05, row['Volatility_CV'], f"ID:{row['Sample_ID']}", fontsize=9)

    # plt.title('LabelScan-Go 2D Privacy Risk Map', fontsize=14, fontweight='bold')
    # plt.xlabel(r'Mean Decision Distance ($\bar{d}$)', fontsize=12)
    # plt.ylabel(r'Landscape Volatility Coefficient ($CV$)', fontsize=12)
    # plt.legend()
    # plt.tight_layout()
    # plt.savefig(args.plot, dpi=300)
if __name__ == "__main__":
    main()
