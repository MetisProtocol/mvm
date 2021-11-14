// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

interface iMVM_DiscountOracle{

    function setDiscount(
        uint256 _discount
    ) external;
    
    function setMinL2Gas(
        uint256 _minL2Gas
    ) external;
    
    function setWhitelistedXDomainSender(
        address _sender,
        bool _isWhitelisted
    ) external;
    
    function isXDomainSenderAllowed(
        address _sender
    ) view external returns(bool);
    
    function setAllowAllXDomainSenders(
        bool _allowAllXDomainSenders
    ) external;
    
    function getMinL2Gas() view external returns(uint256);
    function getDiscount() view external returns(uint256);
    function processL2SeqGas(address sender, uint256 _chainId) external payable;
}
