/// @title IFaultDisputeLockingPool
/// @notice Interface for the FaultDisputeLockingPool contract.
interface IFaultDisputeLockingPool {
    /// @notice Event emitted when tokens are deposited into the vault.
    event Deposit(address indexed account, uint256 amount);

    /// @notice Event emitted when tokens are withdrawn from the vault.
    event Withdrawal(address indexed account, uint256 amount);

    /// @notice Allows a user to deposit tokens into the vault.
    /// @param amount The amount of tokens to deposit.
    function deposit(uint256 amount) external;

    /// @notice Allows a user to initiate an unlock request for delayed withdrawal.
    /// @param beneficiary The address that will receive the tokens upon withdrawal.
    /// @param amount The amount of tokens to unlock for withdrawal.
    function unlock(address beneficiary, uint256 amount) external;

    /// @notice Allows a user to withdraw unlocked tokens after the delay period.
    /// @param amount The amount of tokens to withdraw.
    function withdraw(uint256 amount) external;

    /// @notice Allows a user to withdraw unlocked tokens to a specific beneficiary after the delay period.
    /// @param beneficiary The address receiving the withdrawn tokens.
    /// @param amount The amount of tokens to withdraw.
    function withdraw(address beneficiary, uint256 amount) external;
}
